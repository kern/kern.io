/**
 * Offline mesh clusterization.
 *
 * Takes raw triangle soup and produces:
 *   1. Meshlets / clusters of ~64-128 triangles
 *   2. Per-cluster bounding sphere + normal cone
 *   3. Packed vertex/index buffers
 *
 * Algorithm: greedy spatial clustering using an adjacency graph.
 */

import { Vec3, v3add, v3sub, v3scale, v3normalize, v3len, v3cross, v3dot } from './math';
import type { Cluster, Page, MeshDescriptor } from './types';
import {
  TARGET_CLUSTER_TRIANGLES,
  CLUSTERS_PER_PAGE,
  VERTEX_STRIDE_FLOATS,
  LEAF_ERROR_RADIUS_FRACTION,
} from './constants';

export interface RawMesh {
  positions: Float32Array; // xyz interleaved
  normals: Float32Array;   // xyz interleaved
  uvs: Float32Array;       // uv interleaved
  indices: Uint32Array;
}

export interface ClusterBuildResult {
  clusters: Cluster[];
  pages: Page[];
  mesh: MeshDescriptor;
  vertexData: Float32Array;
  indexData: Uint32Array;
}

// ─── Build adjacency ────────────────────────────────────────────────────────

interface Adjacency {
  /** For each triangle, list of adjacent triangle indices. */
  neighbors: number[][];
}

function buildAdjacency(indices: Uint32Array, triCount: number): Adjacency {
  // Edge → triangle list
  const edgeMap = new Map<string, number[]>();
  const neighbors: number[][] = new Array(triCount);
  for (let i = 0; i < triCount; i++) neighbors[i] = [];

  for (let t = 0; t < triCount; t++) {
    const i0 = indices[t * 3];
    const i1 = indices[t * 3 + 1];
    const i2 = indices[t * 3 + 2];
    const edges = [[i0, i1], [i1, i2], [i2, i0]];
    for (const [a, b] of edges) {
      const key = a < b ? `${a}_${b}` : `${b}_${a}`;
      let list = edgeMap.get(key);
      if (!list) { list = []; edgeMap.set(key, list); }
      list.push(t);
    }
  }

  for (const tris of edgeMap.values()) {
    for (let i = 0; i < tris.length; i++) {
      for (let j = i + 1; j < tris.length; j++) {
        if (!neighbors[tris[i]].includes(tris[j])) {
          neighbors[tris[i]].push(tris[j]);
        }
        if (!neighbors[tris[j]].includes(tris[i])) {
          neighbors[tris[j]].push(tris[i]);
        }
      }
    }
  }

  return { neighbors };
}

// ─── Greedy spatial clustering ──────────────────────────────────────────────

function clusterTriangles(
  positions: Float32Array,
  indices: Uint32Array,
  triCount: number,
): number[][] {
  const adj = buildAdjacency(indices, triCount);
  const assigned = new Uint8Array(triCount);
  const clusters: number[][] = [];

  // Pre-mark degenerate (zero-area) triangles so they are excluded from all
  // clusters.  A triangle is degenerate when two vertices share the same
  // position (e.g. the polar fan triangles of a UV sphere), making the cross
  // product of its edges near-zero.  These triangles contribute nothing to the
  // rendered image, and their zero-length face normals would corrupt the normal
  // cone computation, disabling backface culling for the cluster.
  for (let t = 0; t < triCount; t++) {
    const i0 = indices[t * 3], i1 = indices[t * 3 + 1], i2 = indices[t * 3 + 2];
    const e1x = positions[i1 * 3] - positions[i0 * 3];
    const e1y = positions[i1 * 3 + 1] - positions[i0 * 3 + 1];
    const e1z = positions[i1 * 3 + 2] - positions[i0 * 3 + 2];
    const e2x = positions[i2 * 3] - positions[i0 * 3];
    const e2y = positions[i2 * 3 + 1] - positions[i0 * 3 + 1];
    const e2z = positions[i2 * 3 + 2] - positions[i0 * 3 + 2];
    const cx = e1y * e2z - e1z * e2y;
    const cy = e1z * e2x - e1x * e2z;
    const cz = e1x * e2y - e1y * e2x;
    if (cx * cx + cy * cy + cz * cz < 1e-14) assigned[t] = 1;
  }

  // Compute triangle centroids
  const centroids = new Float32Array(triCount * 3);
  for (let t = 0; t < triCount; t++) {
    const i0 = indices[t * 3], i1 = indices[t * 3 + 1], i2 = indices[t * 3 + 2];
    centroids[t * 3] = (positions[i0 * 3] + positions[i1 * 3] + positions[i2 * 3]) / 3;
    centroids[t * 3 + 1] = (positions[i0 * 3 + 1] + positions[i1 * 3 + 1] + positions[i2 * 3 + 1]) / 3;
    centroids[t * 3 + 2] = (positions[i0 * 3 + 2] + positions[i1 * 3 + 2] + positions[i2 * 3 + 2]) / 3;
  }

  for (let seed = 0; seed < triCount; seed++) {
    if (assigned[seed]) continue;

    const cluster: number[] = [seed];
    assigned[seed] = 1;
    const frontier: number[] = [...adj.neighbors[seed]];

    while (cluster.length < TARGET_CLUSTER_TRIANGLES && frontier.length > 0) {
      // Pick the frontier triangle closest to cluster centroid
      let bestIdx = -1;
      let bestDist = Infinity;
      const cx = centroids[seed * 3], cy = centroids[seed * 3 + 1], cz = centroids[seed * 3 + 2];

      for (let i = 0; i < frontier.length; i++) {
        const t = frontier[i];
        if (assigned[t]) continue;
        const dx = centroids[t * 3] - cx;
        const dy = centroids[t * 3 + 1] - cy;
        const dz = centroids[t * 3 + 2] - cz;
        const d = dx * dx + dy * dy + dz * dz;
        if (d < bestDist) { bestDist = d; bestIdx = i; }
      }

      if (bestIdx === -1) break;
      const chosen = frontier[bestIdx];
      frontier[bestIdx] = frontier[frontier.length - 1];
      frontier.pop();

      if (assigned[chosen]) continue;
      assigned[chosen] = 1;
      cluster.push(chosen);

      for (const n of adj.neighbors[chosen]) {
        if (!assigned[n]) frontier.push(n);
      }
    }

    clusters.push(cluster);
  }

  return clusters;
}

// ─── Compute bounding sphere (Ritter's algorithm) ───────────────────────────

function computeBoundingSphere(
  positions: Float32Array,
  vertexIndices: Set<number>,
): Float32Array {
  const verts = Array.from(vertexIndices);
  if (verts.length === 0) return new Float32Array([0, 0, 0, 0]);

  let p0: Vec3 = [positions[verts[0] * 3], positions[verts[0] * 3 + 1], positions[verts[0] * 3 + 2]];

  // Find farthest from p0
  let maxDist = 0;
  let p1 = p0;
  for (const vi of verts) {
    const p: Vec3 = [positions[vi * 3], positions[vi * 3 + 1], positions[vi * 3 + 2]];
    const d = v3len(v3sub(p, p0));
    if (d > maxDist) { maxDist = d; p1 = p; }
  }

  // Find farthest from p1
  maxDist = 0;
  let p2 = p1;
  for (const vi of verts) {
    const p: Vec3 = [positions[vi * 3], positions[vi * 3 + 1], positions[vi * 3 + 2]];
    const d = v3len(v3sub(p, p1));
    if (d > maxDist) { maxDist = d; p2 = p; }
  }

  let center: Vec3 = v3scale(v3add(p1, p2), 0.5);
  let radius = v3len(v3sub(p1, p2)) * 0.5;

  // Expand to include all points
  for (const vi of verts) {
    const p: Vec3 = [positions[vi * 3], positions[vi * 3 + 1], positions[vi * 3 + 2]];
    const d = v3len(v3sub(p, center));
    if (d > radius) {
      radius = (radius + d) * 0.5;
      const offset = d - radius;
      center = v3add(center, v3scale(v3normalize(v3sub(p, center)), offset));
    }
  }

  return new Float32Array([center[0], center[1], center[2], radius]);
}

// ─── Normal cone ────────────────────────────────────────────────────────────

function computeNormalCone(
  positions: Float32Array,
  indices: Uint32Array,
  triIndices: number[],
): Float32Array {
  if (triIndices.length === 0) return new Float32Array([0, 0, 1, 1]);

  // Compute average normal — skip degenerate triangles (zero-area, zero cross product)
  let avg: Vec3 = [0, 0, 0];
  for (const t of triIndices) {
    const i0 = indices[t * 3], i1 = indices[t * 3 + 1], i2 = indices[t * 3 + 2];
    const v0: Vec3 = [positions[i0 * 3], positions[i0 * 3 + 1], positions[i0 * 3 + 2]];
    const v1: Vec3 = [positions[i1 * 3], positions[i1 * 3 + 1], positions[i1 * 3 + 2]];
    const v2: Vec3 = [positions[i2 * 3], positions[i2 * 3 + 1], positions[i2 * 3 + 2]];
    const n = v3normalize(v3cross(v3sub(v1, v0), v3sub(v2, v0)));
    if (v3len(n) < 0.5) continue; // degenerate triangle — cross product is near-zero
    avg = v3add(avg, n);
  }
  avg = v3normalize(avg);
  if (v3len(avg) < 0.001) return new Float32Array([0, 0, 1, -1]); // degenerate → no culling

  // Find max deviation — skip degenerate triangles for the same reason
  let minCos = 1.0;
  for (const t of triIndices) {
    const i0 = indices[t * 3], i1 = indices[t * 3 + 1], i2 = indices[t * 3 + 2];
    const v0: Vec3 = [positions[i0 * 3], positions[i0 * 3 + 1], positions[i0 * 3 + 2]];
    const v1: Vec3 = [positions[i1 * 3], positions[i1 * 3 + 1], positions[i1 * 3 + 2]];
    const v2: Vec3 = [positions[i2 * 3], positions[i2 * 3 + 1], positions[i2 * 3 + 2]];
    const n = v3normalize(v3cross(v3sub(v1, v0), v3sub(v2, v0)));
    if (v3len(n) < 0.5) continue; // degenerate triangle
    const d = v3dot(n, avg);
    if (d < minCos) minCos = d;
  }

  return new Float32Array([avg[0], avg[1], avg[2], minCos]);
}

// ─── Compute geometric error ────────────────────────────────────────────────

/** Error = max distance from cluster boundary to the "ideal" surface. */
function computeClusterError(boundingSphere: Float32Array): number {
  // For leaf clusters, geometric error is proportional to cluster size.
  // A more accurate metric would compare to the original mesh,
  // but for the first pass, radius/10 is a reasonable proxy.
  return boundingSphere[3] * LEAF_ERROR_RADIUS_FRACTION;
}

// ─── Pack geometry ──────────────────────────────────────────────────────────

interface PackedClusterGeometry {
  vertexData: Float32Array; // 8 floats per vert (pos3 + normal3 + uv2)
  indexData: Uint32Array;
  vertexCount: number;
  indexCount: number;
}

function packClusterGeometry(
  positions: Float32Array,
  normals: Float32Array,
  uvs: Float32Array,
  indices: Uint32Array,
  triIndices: number[],
): PackedClusterGeometry {
  const vertexSet = new Set<number>();
  for (const t of triIndices) {
    vertexSet.add(indices[t * 3]);
    vertexSet.add(indices[t * 3 + 1]);
    vertexSet.add(indices[t * 3 + 2]);
  }

  const globalToLocal = new Map<number, number>();
  const vertexArray: number[] = [];
  let localIdx = 0;

  for (const gi of vertexSet) {
    globalToLocal.set(gi, localIdx++);
    vertexArray.push(
      positions[gi * 3], positions[gi * 3 + 1], positions[gi * 3 + 2],
      normals[gi * 3], normals[gi * 3 + 1], normals[gi * 3 + 2],
      uvs[gi * 2], uvs[gi * 2 + 1],
    );
  }

  const indexArray: number[] = [];
  for (const t of triIndices) {
    indexArray.push(
      globalToLocal.get(indices[t * 3])!,
      globalToLocal.get(indices[t * 3 + 1])!,
      globalToLocal.get(indices[t * 3 + 2])!,
    );
  }

  return {
    vertexData: new Float32Array(vertexArray),
    indexData: new Uint32Array(indexArray),
    vertexCount: vertexSet.size,
    indexCount: indexArray.length,
  };
}

// ─── Main build entry ───────────────────────────────────────────────────────

export function buildClusters(
  raw: RawMesh,
  meshId: number,
  meshName: string,
  globalClusterOffset: number,
  globalVertexOffset: number,
  globalIndexOffset: number,
): ClusterBuildResult {
  const triCount = raw.indices.length / 3;

  // Step 1: partition triangles into clusters
  const triClusters = clusterTriangles(raw.positions, raw.indices, triCount);

  // Step 2: for each cluster, pack geometry and compute metadata
  const clusters: Cluster[] = [];
  const allVertexData: Float32Array[] = [];
  const allIndexData: Uint32Array[] = [];
  let vOffset = globalVertexOffset;
  let iOffset = globalIndexOffset;

  for (let ci = 0; ci < triClusters.length; ci++) {
    const triList = triClusters[ci];
    const packed = packClusterGeometry(
      raw.positions, raw.normals, raw.uvs, raw.indices, triList,
    );

    const vertexIndices = new Set<number>();
    for (const t of triList) {
      vertexIndices.add(raw.indices[t * 3]);
      vertexIndices.add(raw.indices[t * 3 + 1]);
      vertexIndices.add(raw.indices[t * 3 + 2]);
    }

    const bs = computeBoundingSphere(raw.positions, vertexIndices);
    const nc = computeNormalCone(raw.positions, raw.indices, triList);
    const err = computeClusterError(bs);

    clusters.push({
      boundingSphere: bs,
      normalCone: nc,
      lodError: err,
      parentIndex: -1,
      childOffset: -1,
      childCount: 0,
      vertexOffset: vOffset,
      vertexCount: packed.vertexCount,
      indexOffset: iOffset,
      indexCount: packed.indexCount,
      materialId: 0,
      pageId: -1, // assigned later
      lodLevel: 0,
    });

    allVertexData.push(packed.vertexData);
    allIndexData.push(packed.indexData);
    vOffset += packed.vertexCount * VERTEX_STRIDE_FLOATS;
    iOffset += packed.indexCount;
  }

  // Step 3: build pages
  const pages: Page[] = [];
  for (let i = 0; i < clusters.length; i += CLUSTERS_PER_PAGE) {
    const end = Math.min(i + CLUSTERS_PER_PAGE, clusters.length);
    const clusterIds: number[] = [];
    let pvStart = Infinity, pvEnd = 0, piStart = Infinity, piEnd = 0;
    for (let j = i; j < end; j++) {
      clusterIds.push(globalClusterOffset + j);
      clusters[j].pageId = pages.length;
      const c = clusters[j];
      pvStart = Math.min(pvStart, c.vertexOffset);
      pvEnd = Math.max(pvEnd, c.vertexOffset + c.vertexCount * VERTEX_STRIDE_FLOATS);
      piStart = Math.min(piStart, c.indexOffset);
      piEnd = Math.max(piEnd, c.indexOffset + c.indexCount);
    }
    pages.push({
      id: pages.length,
      vertexBufferOffset: pvStart,
      vertexBufferSize: (pvEnd - pvStart) * 4,
      indexBufferOffset: piStart,
      indexBufferSize: (piEnd - piStart) * 4,
      clusterIds,
    });
  }

  // Step 4: concatenate vertex/index data
  let totalVerts = 0, totalIdx = 0;
  for (const vd of allVertexData) totalVerts += vd.length;
  for (const id of allIndexData) totalIdx += id.length;
  const vertexData = new Float32Array(totalVerts);
  const indexData = new Uint32Array(totalIdx);
  let vo = 0, io = 0;
  for (const vd of allVertexData) { vertexData.set(vd, vo); vo += vd.length; }
  for (const id of allIndexData) { indexData.set(id, io); io += id.length; }

  const mesh: MeshDescriptor = {
    id: meshId,
    name: meshName,
    clusterOffset: globalClusterOffset,
    clusterCount: clusters.length,
    rootCluster: globalClusterOffset, // will be updated by hierarchy builder
    pageIds: pages.map(p => p.id),
    totalVertices: totalVerts / 8,
    totalIndices: totalIdx,
    totalTriangles: triCount,
  };

  return { clusters, pages, mesh, vertexData, indexData };
}
