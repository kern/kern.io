/**
 * Builds a cluster hierarchy (DAG) for LOD selection.
 *
 * Implements the key ideas from Nanite's cluster hierarchy:
 *
 *  1. Greedy nearest-neighbour grouping (approximates METIS graph
 *     partitioning to minimise shared boundary edges between groups).
 *
 *  2. Boundary-locked edge collapse for parent geometry generation:
 *     - Weld coincident vertices across child cluster boundaries so the
 *       merged group has a proper connected mesh.
 *     - Mark boundary vertices (on edges that border adjacent groups, i.e.
 *       edges shared by exactly one triangle in the merged mesh) as locked.
 *     - Iteratively collapse the cheapest unlocked edge (shortest edge
 *       length as a proxy for QEM cost) until ~50 % of triangles remain.
 *     - Because boundary vertices never move, adjacent parent clusters share
 *       identical boundary positions — no T-junction cracks.
 *
 * The hierarchy allows GPU-driven top-down traversal:
 *   start at root → test error → refine to children or accept.
 */

import type { Cluster, Page } from './types';
import { Vec3, v3add, v3sub, v3len, v3normalize, v3dot } from './math';
import {
  MAX_CHILDREN_PER_PARENT,
  MIN_CLUSTERS_FOR_HIERARCHY,
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

// ─── Bounding-sphere / normal-cone helpers ───────────────────────────────────

function mergeBoundingSpheres(spheres: Float32Array[]): Float32Array {
  if (spheres.length === 0) return new Float32Array([0, 0, 0, 0]);
  if (spheres.length === 1) return new Float32Array(spheres[0]);

  let cx = 0, cy = 0, cz = 0;
  for (const s of spheres) { cx += s[0]; cy += s[1]; cz += s[2]; }
  cx /= spheres.length; cy /= spheres.length; cz /= spheres.length;

  let maxR = 0;
  for (const s of spheres) {
    const dx = s[0] - cx, dy = s[1] - cy, dz = s[2] - cz;
    const d = Math.sqrt(dx * dx + dy * dy + dz * dz) + s[3];
    if (d > maxR) maxR = d;
  }
  return new Float32Array([cx, cy, cz, maxR]);
}

function mergeNormalCones(cones: Float32Array[]): Float32Array {
  if (cones.length === 0) return new Float32Array([0, 0, 1, -1]);

  let dx = 0, dy = 0, dz = 0;
  for (const c of cones) { dx += c[0]; dy += c[1]; dz += c[2]; }
  const len = Math.sqrt(dx * dx + dy * dy + dz * dz);
  if (len < 0.001) return new Float32Array([0, 0, 1, -1]);
  dx /= len; dy /= len; dz /= len;

  let minCos = 1.0;
  for (const c of cones) {
    const dot = c[0] * dx + c[1] * dy + c[2] * dz;
    const childHalfAngle = Math.acos(Math.max(-1, Math.min(1, c[3])));
    const deviation = Math.acos(Math.max(-1, Math.min(1, dot)));
    const totalAngle = deviation + childHalfAngle;
    const totalCos = Math.cos(Math.min(Math.PI, totalAngle));
    if (totalCos < minCos) minCos = totalCos;
  }
  return new Float32Array([dx, dy, dz, minCos]);
}

// ─── Greedy nearest-neighbour grouping ───────────────────────────────────────

/**
 * Group cluster indices into batches of up to `maxPerGroup`.
 *
 * Uses greedy nearest-neighbour: pick the first ungrouped cluster, then
 * repeatedly add the closest ungrouped neighbour until the group is full or
 * all clusters are consumed. This minimises the total boundary length between
 * groups, approximating the METIS graph-partitioning approach Nanite uses.
 */
function greedyGroup(
  indices: number[],
  clusters: Cluster[],
  maxPerGroup: number,
): number[][] {
  const remaining = new Set(indices);
  const groups: number[][] = [];

  while (remaining.size > 0) {
    // Seed: pick the first remaining (stable ordering)
    const [seed] = remaining;
    remaining.delete(seed);
    const group = [seed];
    const sc = clusters[seed].boundingSphere;

    while (group.length < maxPerGroup && remaining.size > 0) {
      let bestDist = Infinity;
      let bestIdx = -1;
      for (const ci of remaining) {
        const cc = clusters[ci].boundingSphere;
        const dx = cc[0] - sc[0], dy = cc[1] - sc[1], dz = cc[2] - sc[2];
        const d = dx * dx + dy * dy + dz * dz;
        if (d < bestDist) { bestDist = d; bestIdx = ci; }
      }
      remaining.delete(bestIdx);
      group.push(bestIdx);
    }

    groups.push(group);
  }

  return groups;
}

// ─── Nanite-style parent geometry: weld → lock boundary → edge collapse ──────

/**
 * Generate a simplified (LOD) representation for a parent cluster.
 *
 * Algorithm (Nanite-inspired):
 *   1. Merge all child triangles into a unified soup.
 *   2. Weld vertices that share the same world position (merging the seams
 *      between child clusters so the mesh has proper connectivity).
 *   3. Identify boundary vertices — vertices on edges that are referenced by
 *      exactly one triangle (i.e. the group's open boundary with its
 *      neighbours).  These are LOCKED and will not be moved.
 *   4. Iteratively collapse the cheapest (shortest) non-locked, non-boundary
 *      edge until ~50 % of the original triangles remain.
 *      Moving the kept vertex to the midpoint of the collapsed edge gives
 *      better approximation quality than a pure half-edge collapse.
 *   5. Pack into a compact vertex + index buffer.
 */
function generateParentGeometry(
  children: Cluster[],
  allVertexData: Float32Array,
  allIndexData: Uint32Array,
  vertexStride: number,
  vertexDataOffset: number,
  indexDataOffset: number,
): { vertexData: Float32Array; indexData: Uint32Array } {
  // ── 1. Collect all child vertices and triangles ──────────────────────────
  // rawVerts: one flat entry per child-local vertex [x,y,z, nx,ny,nz, u,v]
  // rawTris: global (across children) vertex indices
  const rawVerts: number[][] = [];
  const rawTris: [number, number, number][] = [];

  for (const child of children) {
    // cluster.vertexOffset / indexOffset are global; subtract the base so we
    // index into the local allVertexData / allIndexData buffers correctly.
    const vBase = child.vertexOffset - vertexDataOffset;
    const iBase = child.indexOffset - indexDataOffset;
    const triCount = child.indexCount / 3;
    const vertOffset = rawVerts.length;

    for (let v = 0; v < child.vertexCount; v++) {
      const off = vBase + v * vertexStride;
      rawVerts.push([
        allVertexData[off],     allVertexData[off + 1], allVertexData[off + 2],
        allVertexData[off + 3], allVertexData[off + 4], allVertexData[off + 5],
        allVertexData[off + 6], allVertexData[off + 7],
      ]);
    }

    for (let t = 0; t < triCount; t++) {
      const i0 = allIndexData[iBase + t * 3]     + vertOffset;
      const i1 = allIndexData[iBase + t * 3 + 1] + vertOffset;
      const i2 = allIndexData[iBase + t * 3 + 2] + vertOffset;
      rawTris.push([i0, i1, i2]);
    }
  }

  // ── 2. Weld coincident positions ─────────────────────────────────────────
  // Grid key: quantise to 5 decimal places (~0.01 mm precision).
  const posKey = (v: number[]) =>
    `${v[0].toFixed(5)},${v[1].toFixed(5)},${v[2].toFixed(5)}`;

  const posMap = new Map<string, number>();
  const weldedVerts: number[][] = [];
  const remap = new Int32Array(rawVerts.length);

  for (let i = 0; i < rawVerts.length; i++) {
    const k = posKey(rawVerts[i]);
    if (posMap.has(k)) {
      remap[i] = posMap.get(k)!;
    } else {
      const idx = weldedVerts.length;
      posMap.set(k, idx);
      weldedVerts.push([...rawVerts[i]]);
      remap[i] = idx;
    }
  }

  // Remap and remove degenerate triangles.
  let tris: ([number, number, number] | null)[] = rawTris
    .map(([a, b, c]): [number, number, number] => [remap[a], remap[b], remap[c]])
    .filter((t): t is [number, number, number] => t[0] !== t[1] && t[1] !== t[2] && t[0] !== t[2]);

  if (tris.length === 0) return fallbackGeometry(children, allVertexData, allIndexData, vertexStride, vertexDataOffset, indexDataOffset);

  // ── 3. Find boundary vertices (open boundary edges) ──────────────────────
  const edgeUses = new Map<string, number>();
  const edgeKey = (a: number, b: number) => a < b ? `${a},${b}` : `${b},${a}`;

  for (const t of tris) {
    if (!t) continue;
    const [a, b, c] = t;
    edgeUses.set(edgeKey(a, b), (edgeUses.get(edgeKey(a, b)) ?? 0) + 1);
    edgeUses.set(edgeKey(b, c), (edgeUses.get(edgeKey(b, c)) ?? 0) + 1);
    edgeUses.set(edgeKey(c, a), (edgeUses.get(edgeKey(c, a)) ?? 0) + 1);
  }

  const locked = new Uint8Array(weldedVerts.length);
  for (const [k, count] of edgeUses) {
    if (count === 1) {
      const comma = k.indexOf(',');
      locked[parseInt(k, 10)] = 1;
      locked[parseInt(k.slice(comma + 1), 10)] = 1;
    }
  }

  // ── 4. Edge-collapse to ~50 % of original triangle count ─────────────────
  const verts = weldedVerts; // mutated in place
  const triArr: ([number, number, number] | null)[] = [...tris];
  const targetTris = Math.max(3, Math.ceil(triArr.length / 2));

  let activeTris = triArr.length;

  while (activeTris > targetTris) {
    // Scan all edges, find cheapest non-locked, non-boundary candidate.
    let bestCost = Infinity;
    let bestKeep = -1, bestRemove = -1;
    const seen = new Set<string>();

    for (const t of triArr) {
      if (!t) continue;
      const [a, b, c] = t;
      for (const [p, q] of [[a, b], [b, c], [c, a]] as [number, number][]) {
        if (locked[p] || locked[q]) continue;
        const k = edgeKey(p, q);
        if (seen.has(k)) continue;
        seen.add(k);
        const dx = verts[p][0] - verts[q][0];
        const dy = verts[p][1] - verts[q][1];
        const dz = verts[p][2] - verts[q][2];
        const cost = dx * dx + dy * dy + dz * dz;
        if (cost < bestCost) {
          bestCost = cost;
          // Keep the vertex with the lower index for determinism.
          bestKeep = Math.min(p, q);
          bestRemove = Math.max(p, q);
        }
      }
    }

    if (bestKeep === -1) break; // no more collapsible edges

    // Move kept vertex to the midpoint (better approximation than half-edge).
    verts[bestKeep][0] = (verts[bestKeep][0] + verts[bestRemove][0]) * 0.5;
    verts[bestKeep][1] = (verts[bestKeep][1] + verts[bestRemove][1]) * 0.5;
    verts[bestKeep][2] = (verts[bestKeep][2] + verts[bestRemove][2]) * 0.5;
    // Average and renormalise normal.
    const nx = verts[bestKeep][3] + verts[bestRemove][3];
    const ny = verts[bestKeep][4] + verts[bestRemove][4];
    const nz = verts[bestKeep][5] + verts[bestRemove][5];
    const nlen = Math.sqrt(nx * nx + ny * ny + nz * nz);
    if (nlen > 0.001) {
      verts[bestKeep][3] = nx / nlen;
      verts[bestKeep][4] = ny / nlen;
      verts[bestKeep][5] = nz / nlen;
    }
    // Average UVs.
    verts[bestKeep][6] = (verts[bestKeep][6] + verts[bestRemove][6]) * 0.5;
    verts[bestKeep][7] = (verts[bestKeep][7] + verts[bestRemove][7]) * 0.5;

    // Redirect all references from bestRemove → bestKeep, remove degenerates.
    for (let i = 0; i < triArr.length; i++) {
      const t = triArr[i];
      if (!t) continue;
      let [a, b, c] = t;
      if (a === bestRemove) a = bestKeep;
      if (b === bestRemove) b = bestKeep;
      if (c === bestRemove) c = bestKeep;
      if (a === b || b === c || a === c) {
        triArr[i] = null;
        activeTris--;
      } else {
        triArr[i] = [a, b, c];
      }
    }
  }

  // ── 5. Pack into compact vertex + index buffer ────────────────────────────
  const finalTris = triArr.filter((t): t is [number, number, number] => t !== null);
  if (finalTris.length === 0) return fallbackGeometry(children, allVertexData, allIndexData, vertexStride, vertexDataOffset, indexDataOffset);

  const usedVerts = new Set<number>();
  for (const [a, b, c] of finalTris) { usedVerts.add(a); usedVerts.add(b); usedVerts.add(c); }

  const vertRemap = new Int32Array(verts.length).fill(-1);
  const outVerts: number[] = [];
  for (const vi of usedVerts) {
    vertRemap[vi] = outVerts.length / vertexStride;
    for (let f = 0; f < vertexStride; f++) outVerts.push(verts[vi][f]);
  }

  const outIndices: number[] = [];
  for (const [a, b, c] of finalTris) {
    outIndices.push(vertRemap[a], vertRemap[b], vertRemap[c]);
  }

  return {
    vertexData: new Float32Array(outVerts),
    indexData: new Uint32Array(outIndices),
  };
}

/** Return the first triangle of the first child verbatim (last-resort fallback). */
function fallbackGeometry(
  children: Cluster[],
  allVertexData: Float32Array,
  allIndexData: Uint32Array,
  vertexStride: number,
  vertexDataOffset: number,
  indexDataOffset: number,
): { vertexData: Float32Array; indexData: Uint32Array } {
  const vBase = children[0].vertexOffset - vertexDataOffset;
  const iBase = children[0].indexOffset - indexDataOffset;
  const fallback: number[] = [];
  for (let v = 0; v < 3; v++) {
    const li = allIndexData[iBase + v];
    const off = vBase + li * vertexStride;
    for (let f = 0; f < vertexStride; f++) fallback.push(allVertexData[off + f]);
  }
  return {
    vertexData: new Float32Array(fallback),
    indexData: new Uint32Array([0, 1, 2]),
  };
}

// ─── Main build entry ─────────────────────────────────────────────────────────

/**
 * Build the cluster hierarchy.
 *
 * Input:  leaf clusters from buildClusters.
 * Output: extended cluster array with parent nodes, updated links.
 */
export function buildHierarchy(
  leafClusters: Cluster[],
  pages: Page[],
  vertexData: Float32Array,
  indexData: Uint32Array,
  globalClusterOffset: number = 0,
  globalVertexFloatOffset: number = 0,
  globalIndexOffset: number = 0,
): HierarchyBuildResult {
  if (leafClusters.length < MIN_CLUSTERS_FOR_HIERARCHY) {
    return {
      clusters: leafClusters,
      pages,
      rootIndex: globalClusterOffset,
      parentVertexData: new Float32Array(0),
      parentIndexData: new Uint32Array(0),
    };
  }

  const VERTEX_STRIDE = VERTEX_STRIDE_FLOATS;
  const allClusters = [...leafClusters];
  const allPages = [...pages];
  const parentVertexParts: Float32Array[] = [];
  const parentIndexParts: Uint32Array[] = [];
  // Parent cluster offsets are global (= globalVertexFloatOffset + bytes into
  // the combined local buffer that starts with vertexData).
  let parentVertexFloatOffset = globalVertexFloatOffset + vertexData.length;
  let parentIndexOffset = globalIndexOffset + indexData.length;

  // combinedVertexData / combinedIndexData are LOCAL buffers (start at 0).
  // All cluster reads must subtract globalVertexFloatOffset / globalIndexOffset
  // before indexing into these buffers.
  let combinedVertexData = vertexData;
  let combinedIndexData = indexData;

  let currentLevel = leafClusters.map((_, i) => i);

  while (currentLevel.length > 1) {
    const nextLevel: number[] = [];

    // Group by greedy nearest-neighbour to minimise boundary edge count.
    const groups = greedyGroup(currentLevel, allClusters, MAX_CHILDREN_PER_PARENT);

    for (const childIndices of groups) {
      if (childIndices.length === 1 && groups.length === 1) {
        // Already a single root — don't wrap it.
        nextLevel.push(childIndices[0]);
        continue;
      }

      const childClusters = childIndices.map(ci => allClusters[ci]);
      const bs = mergeBoundingSpheres(childClusters.map(c => c.boundingSphere));
      const nc = mergeNormalCones(childClusters.map(c => c.normalCone));

      const maxChildError = Math.max(...childClusters.map(c => c.lodError));
      const parentError = maxChildError * PARENT_ERROR_SCALE;

      const parentGeo = generateParentGeometry(
        childClusters, combinedVertexData, combinedIndexData, VERTEX_STRIDE,
        globalVertexFloatOffset, globalIndexOffset,
      );

      const parentIdx = allClusters.length;

      const parent: Cluster = {
        boundingSphere: bs,
        normalCone: nc,
        lodError: parentError,
        parentIndex: -1,
        childOffset: globalClusterOffset + childIndices[0],
        childCount: childIndices.length,
        vertexOffset: parentVertexFloatOffset,
        vertexCount: parentGeo.vertexData.length / VERTEX_STRIDE,
        indexOffset: parentIndexOffset,
        indexCount: parentGeo.indexData.length,
        materialId: childClusters[0].materialId,
        pageId: -1,
        lodLevel: (childClusters[0].lodLevel ?? 0) + 1,
      };

      for (const ci of childIndices) {
        allClusters[ci].parentIndex = globalClusterOffset + parentIdx;
      }

      allClusters.push(parent);
      parentVertexParts.push(parentGeo.vertexData);
      parentIndexParts.push(parentGeo.indexData);
      parentVertexFloatOffset += parentGeo.vertexData.length;
      parentIndexOffset += parentGeo.indexData.length;

      // Extend the combined buffers so the next level can read this parent's geometry.
      const newVD = new Float32Array(combinedVertexData.length + parentGeo.vertexData.length);
      newVD.set(combinedVertexData);
      newVD.set(parentGeo.vertexData, combinedVertexData.length);
      combinedVertexData = newVD;

      const newID = new Uint32Array(combinedIndexData.length + parentGeo.indexData.length);
      newID.set(combinedIndexData);
      newID.set(parentGeo.indexData, combinedIndexData.length);
      combinedIndexData = newID;

      nextLevel.push(parentIdx);
    }

    currentLevel = nextLevel;
  }

  // Assign pages to parent clusters.
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
      vertexBufferOffset: 0,
      vertexBufferSize: 0,
      indexBufferOffset: 0,
      indexBufferSize: 0,
      clusterIds,
    });
    nextPageId++;
  }

  // Concatenate parent geometry buffers.
  let totalParentVerts = 0, totalParentIdx = 0;
  for (const v of parentVertexParts) totalParentVerts += v.length;
  for (const ix of parentIndexParts) totalParentIdx += ix.length;
  const parentVertexData = new Float32Array(totalParentVerts);
  const parentIndexData = new Uint32Array(totalParentIdx);
  let vo = 0, io = 0;
  for (const v of parentVertexParts) { parentVertexData.set(v, vo); vo += v.length; }
  for (const ix of parentIndexParts) { parentIndexData.set(ix, io); io += ix.length; }

  const rootIndex = globalClusterOffset + currentLevel[0];

  return {
    clusters: allClusters,
    pages: allPages,
    rootIndex,
    parentVertexData,
    parentIndexData,
  };
}
