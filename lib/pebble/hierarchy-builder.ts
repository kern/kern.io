/**
 * Builds a cluster hierarchy (DAG) for LOD selection.
 *
 * Takes leaf clusters from the cluster builder and creates parent nodes
 * by merging spatially close clusters. Each parent has a coarser
 * representation and a larger geometric error.
 *
 * The hierarchy allows GPU-driven top-down traversal:
 *   start at root → test error → refine to children or accept.
 */

import type { Cluster, Page } from './types';
import { Vec3, v3add, v3scale, v3sub, v3len, v3normalize, v3dot } from './math';
import {
  MAX_CHILDREN_PER_PARENT,
  MIN_CLUSTERS_FOR_HIERARCHY,
  PARENT_VERTEX_SUBSAMPLE_STEP,
  PARENT_ERROR_SCALE,
  VERTEX_STRIDE_FLOATS,
  CLUSTERS_PER_PAGE,
} from './constants';

export interface HierarchyBuildResult {
  /** Updated clusters array (leaf + internal nodes) */
  clusters: Cluster[];
  /** Updated pages array */
  pages: Page[];
  /** Index of the root cluster */
  rootIndex: number;
  /** Additional vertex data for parent clusters */
  parentVertexData: Float32Array;
  /** Additional index data for parent clusters */
  parentIndexData: Uint32Array;
}

/** Merge bounding spheres. */
function mergeBoundingSpheres(spheres: Float32Array[]): Float32Array {
  if (spheres.length === 0) return new Float32Array([0, 0, 0, 0]);
  if (spheres.length === 1) return new Float32Array(spheres[0]);

  // Compute encompassing sphere
  let cx = 0, cy = 0, cz = 0;
  for (const s of spheres) {
    cx += s[0]; cy += s[1]; cz += s[2];
  }
  cx /= spheres.length; cy /= spheres.length; cz /= spheres.length;

  let maxR = 0;
  for (const s of spheres) {
    const dx = s[0] - cx, dy = s[1] - cy, dz = s[2] - cz;
    const d = Math.sqrt(dx * dx + dy * dy + dz * dz) + s[3];
    if (d > maxR) maxR = d;
  }

  return new Float32Array([cx, cy, cz, maxR]);
}

/** Merge normal cones. Result is a conservative cone containing all input cones. */
function mergeNormalCones(cones: Float32Array[]): Float32Array {
  if (cones.length === 0) return new Float32Array([0, 0, 1, -1]);

  // Average direction
  let dx = 0, dy = 0, dz = 0;
  for (const c of cones) {
    dx += c[0]; dy += c[1]; dz += c[2];
  }
  const len = Math.sqrt(dx * dx + dy * dy + dz * dz);
  if (len < 0.001) return new Float32Array([0, 0, 1, -1]);
  dx /= len; dy /= len; dz /= len;

  // Find worst-case half-angle
  let minCos = 1.0;
  for (const c of cones) {
    const dot = c[0] * dx + c[1] * dy + c[2] * dz;
    // The half-angle of the child cone adds to the deviation
    const childHalfAngle = Math.acos(Math.max(-1, Math.min(1, c[3])));
    const deviation = Math.acos(Math.max(-1, Math.min(1, dot)));
    const totalAngle = deviation + childHalfAngle;
    const totalCos = Math.cos(Math.min(Math.PI, totalAngle));
    if (totalCos < minCos) minCos = totalCos;
  }

  return new Float32Array([dx, dy, dz, minCos]);
}

/**
 * Generate a simplified representation for a parent cluster.
 * Takes child clusters' geometry and produces a decimated version.
 *
 * For simplicity, we use vertex subsampling (every Nth vertex)
 * rather than full mesh decimation.
 */
function generateParentGeometry(
  children: Cluster[],
  allVertexData: Float32Array,
  allIndexData: Uint32Array,
  vertexStride: number,
): { vertexData: Float32Array; indexData: Uint32Array } {
  // Collect all child vertices and create a simplified version
  // Simple approach: gather all unique positions, subsample
  const positions: number[] = [];
  const normals: number[] = [];
  const uvs: number[] = [];

  for (const child of children) {
    // Read child's vertices from the packed buffer
    const vBase = child.vertexOffset; // float offset
    for (let i = 0; i < child.vertexCount; i++) {
      const off = vBase + i * vertexStride;
      positions.push(allVertexData[off], allVertexData[off + 1], allVertexData[off + 2]);
      normals.push(allVertexData[off + 3], allVertexData[off + 4], allVertexData[off + 5]);
      uvs.push(allVertexData[off + 6], allVertexData[off + 7]);
    }
  }

  // Subsample: keep every 4th vertex, form triangles from remaining
  const step = PARENT_VERTEX_SUBSAMPLE_STEP;
  const sampledPos: number[] = [];
  const sampledNorm: number[] = [];
  const sampledUv: number[] = [];
  const vertCount = positions.length / 3;

  for (let i = 0; i < vertCount; i += step) {
    const pi = i * 3;
    sampledPos.push(positions[pi], positions[pi + 1], positions[pi + 2]);
    sampledNorm.push(normals[pi], normals[pi + 1], normals[pi + 2]);
    const ui = i * 2;
    sampledUv.push(uvs[ui], uvs[ui + 1]);
  }

  const sampledVertCount = sampledPos.length / 3;
  if (sampledVertCount < 3) {
    // Not enough verts for any triangle, return minimal geometry
    return {
      vertexData: new Float32Array([
        ...positions.slice(0, 3), ...normals.slice(0, 3), ...uvs.slice(0, 2),
        ...(positions.length >= 6 ? positions.slice(3, 6) : positions.slice(0, 3)),
        ...(normals.length >= 6 ? normals.slice(3, 6) : normals.slice(0, 3)),
        ...(uvs.length >= 4 ? uvs.slice(2, 4) : uvs.slice(0, 2)),
        ...(positions.length >= 9 ? positions.slice(6, 9) : positions.slice(0, 3)),
        ...(normals.length >= 9 ? normals.slice(6, 9) : normals.slice(0, 3)),
        ...(uvs.length >= 6 ? uvs.slice(4, 6) : uvs.slice(0, 2)),
      ]),
      indexData: new Uint32Array([0, 1, 2]),
    };
  }

  // Build triangles by connecting consecutive sampled vertices
  const parentIndices: number[] = [];
  for (let i = 0; i + 2 < sampledVertCount; i += 3) {
    parentIndices.push(i, i + 1, i + 2);
  }
  // If we have leftover verts, make a last triangle
  if (parentIndices.length === 0 && sampledVertCount >= 3) {
    parentIndices.push(0, 1, 2);
  }

  const vd: number[] = [];
  for (let i = 0; i < sampledVertCount; i++) {
    vd.push(
      sampledPos[i * 3], sampledPos[i * 3 + 1], sampledPos[i * 3 + 2],
      sampledNorm[i * 3], sampledNorm[i * 3 + 1], sampledNorm[i * 3 + 2],
      sampledUv[i * 2], sampledUv[i * 2 + 1],
    );
  }

  return {
    vertexData: new Float32Array(vd),
    indexData: new Uint32Array(parentIndices),
  };
}

/**
 * Build the cluster hierarchy.
 *
 * Input: leaf clusters from buildClusters.
 * Output: extended cluster array with parent nodes, updated links.
 */
export function buildHierarchy(
  leafClusters: Cluster[],
  pages: Page[],
  vertexData: Float32Array,
  indexData: Uint32Array,
  globalClusterOffset: number = 0,
): HierarchyBuildResult {
  if (leafClusters.length < MIN_CLUSTERS_FOR_HIERARCHY) {
    // Too few clusters, just return as-is with a single root
    const rootIdx = 0;
    return {
      clusters: leafClusters,
      pages,
      rootIndex: globalClusterOffset + rootIdx,
      parentVertexData: new Float32Array(0),
      parentIndexData: new Uint32Array(0),
    };
  }

  const VERTEX_STRIDE = VERTEX_STRIDE_FLOATS; // floats per vertex
  const allClusters = [...leafClusters];
  const allPages = [...pages];
  const parentVertexParts: Float32Array[] = [];
  const parentIndexParts: Uint32Array[] = [];
  let parentVertexFloatOffset = vertexData.length;
  let parentIndexOffset = indexData.length;

  // Build levels bottom-up
  let currentLevel = leafClusters.map((_, i) => i); // indices into allClusters

  while (currentLevel.length > 1) {
    const nextLevel: number[] = [];

    // Group clusters spatially (simple: sort by centroid X, then group)
    const sorted = [...currentLevel].sort((a, b) => {
      const ca = allClusters[a].boundingSphere;
      const cb = allClusters[b].boundingSphere;
      return ca[0] - cb[0] || ca[1] - cb[1] || ca[2] - cb[2];
    });

    for (let i = 0; i < sorted.length; i += MAX_CHILDREN_PER_PARENT) {
      const childIndices = sorted.slice(i, Math.min(i + MAX_CHILDREN_PER_PARENT, sorted.length));

      if (childIndices.length === 1 && nextLevel.length === 0 && i + MAX_CHILDREN_PER_PARENT >= sorted.length) {
        // Last single node at top level, just use it as root
        nextLevel.push(childIndices[0]);
        continue;
      }

      // Merge children into a parent
      const childClusters = childIndices.map(ci => allClusters[ci]);
      const bs = mergeBoundingSpheres(childClusters.map(c => c.boundingSphere));
      const nc = mergeNormalCones(childClusters.map(c => c.normalCone));

      // Parent error = max child error * scale factor
      const maxChildError = Math.max(...childClusters.map(c => c.lodError));
      const parentError = maxChildError * PARENT_ERROR_SCALE;

      // Generate simplified geometry for parent
      const parentGeo = generateParentGeometry(
        childClusters, vertexData, indexData, VERTEX_STRIDE,
      );

      const parentIdx = allClusters.length;

      const parent: Cluster = {
        boundingSphere: bs,
        normalCone: nc,
        lodError: parentError,
        parentIndex: -1, // will be set when this level gets a parent
        childOffset: globalClusterOffset + childIndices[0],
        childCount: childIndices.length,
        vertexOffset: parentVertexFloatOffset,
        vertexCount: parentGeo.vertexData.length / VERTEX_STRIDE,
        indexOffset: parentIndexOffset,
        indexCount: parentGeo.indexData.length,
        materialId: childClusters[0].materialId,
        pageId: -1,
        lodLevel: (childClusters[0].lodLevel || 0) + 1,
      };

      // Set parent links on children
      for (const ci of childIndices) {
        allClusters[ci].parentIndex = globalClusterOffset + parentIdx;
      }

      allClusters.push(parent);
      parentVertexParts.push(parentGeo.vertexData);
      parentIndexParts.push(parentGeo.indexData);
      parentVertexFloatOffset += parentGeo.vertexData.length;
      parentIndexOffset += parentGeo.indexData.length;

      nextLevel.push(parentIdx);
    }

    currentLevel = nextLevel;
  }

  // Assign pages to parent clusters
  let nextPageId = allPages.length;
  const PAGE_SIZE = CLUSTERS_PER_PAGE;
  const parentClusters = allClusters.slice(leafClusters.length);
  for (let i = 0; i < parentClusters.length; i += PAGE_SIZE) {
    const end = Math.min(i + PAGE_SIZE, parentClusters.length);
    const clusterIds: number[] = [];
    for (let j = i; j < end; j++) {
      const absIdx = leafClusters.length + j;
      allClusters[absIdx].pageId = nextPageId;
      clusterIds.push(globalClusterOffset + absIdx);
    }
    allPages.push({
      id: nextPageId,
      vertexBufferOffset: 0, // simplified
      vertexBufferSize: 0,
      indexBufferOffset: 0,
      indexBufferSize: 0,
      clusterIds,
    });
    nextPageId++;
  }

  // Concatenate parent geometry
  let totalParentVerts = 0, totalParentIdx = 0;
  for (const v of parentVertexParts) totalParentVerts += v.length;
  for (const i of parentIndexParts) totalParentIdx += i.length;
  const parentVertexData = new Float32Array(totalParentVerts);
  const parentIndexData = new Uint32Array(totalParentIdx);
  let vo = 0, io = 0;
  for (const v of parentVertexParts) { parentVertexData.set(v, vo); vo += v.length; }
  for (const i of parentIndexParts) { parentIndexData.set(i, io); io += i.length; }

  const rootIndex = globalClusterOffset + currentLevel[0];

  return {
    clusters: allClusters,
    pages: allPages,
    rootIndex,
    parentVertexData,
    parentIndexData,
  };
}
