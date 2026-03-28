/**
 * Scene builder: takes raw meshes + instance placements,
 * runs the offline pipeline (clusterize → hierarchy → pages),
 * and produces a BuiltScene ready for GPU upload.
 *
 * Each instance is processed independently: the instance transform is baked
 * into the cluster geometry (positions and normals are transformed into world
 * space at build time).  This means every instance has its own unique set of
 * clusters in the global cluster array, with bounding spheres already in world
 * space.  The vertex shader does not need to apply any per-instance transform.
 */

import type { RawMesh } from './cluster-builder';
import type { BuiltScene, Instance, Cluster, Page, MeshDescriptor } from './types';
import { buildClusters } from './cluster-builder';
import { buildHierarchy } from './hierarchy-builder';
import { mat4Identity, mat4TransformPoint, mat4TransformDir, Mat4, Vec3, v3len, v3sub, v3normalize } from './math';

export interface SceneMeshEntry {
  name: string;
  raw: RawMesh;
  instances: Mat4[]; // list of transforms
}

/**
 * Apply a Mat4 to every vertex in a RawMesh, producing a new RawMesh whose
 * geometry is in world space.  Positions are fully transformed; normals use
 * the upper-3x3 of the matrix (direction transform) and are re-normalised.
 */
function applyTransformToMesh(raw: RawMesh, transform: Mat4): RawMesh {
  const n = raw.positions.length / 3;
  const positions = new Float32Array(n * 3);
  const normals = new Float32Array(n * 3);

  for (let i = 0; i < n; i++) {
    const p: Vec3 = [raw.positions[i * 3], raw.positions[i * 3 + 1], raw.positions[i * 3 + 2]];
    const wp = mat4TransformPoint(transform, p);
    positions[i * 3]     = wp[0];
    positions[i * 3 + 1] = wp[1];
    positions[i * 3 + 2] = wp[2];

    const d: Vec3 = [raw.normals[i * 3], raw.normals[i * 3 + 1], raw.normals[i * 3 + 2]];
    const wn = v3normalize(mat4TransformDir(transform, d));
    normals[i * 3]     = wn[0];
    normals[i * 3 + 1] = wn[1];
    normals[i * 3 + 2] = wn[2];
  }

  return { positions, normals, uvs: raw.uvs, indices: raw.indices };
}

export function buildScene(entries: SceneMeshEntry[]): BuiltScene {
  const allClusters: Cluster[] = [];
  const allPages: Page[] = [];
  const allInstances: Instance[] = [];
  const allMeshes: MeshDescriptor[] = [];
  const vertexParts: Float32Array[] = [];
  const indexParts: Uint32Array[] = [];
  let globalClusterOffset = 0;
  let globalVertexFloatOffset = 0;
  let globalIndexOffset = 0;

  for (let mi = 0; mi < entries.length; mi++) {
    const entry = entries[mi];
    const meshClusterStart = globalClusterOffset;
    let meshClusterCount = 0;
    let meshTotalVerts = 0;
    let meshTotalIndices = 0;
    let meshTotalTriangles = 0;
    const meshPageIds: number[] = [];
    let meshRootCluster = -1;

    for (const transform of entry.instances) {
      // Bake the instance transform into world-space geometry.
      const worldRaw = applyTransformToMesh(entry.raw, transform);

      // Step 1: clusterize the world-space geometry
      const clusterResult = buildClusters(
        worldRaw, mi, entry.name,
        globalClusterOffset, globalVertexFloatOffset, globalIndexOffset,
      );

      // Step 2: build LOD hierarchy
      const hierarchyResult = buildHierarchy(
        clusterResult.clusters,
        clusterResult.pages,
        clusterResult.vertexData,
        clusterResult.indexData,
        globalClusterOffset,
        globalVertexFloatOffset,
        globalIndexOffset,
      );

      // Accumulate
      allClusters.push(...hierarchyResult.clusters);
      allPages.push(...hierarchyResult.pages);
      vertexParts.push(clusterResult.vertexData);
      if (hierarchyResult.parentVertexData.length > 0) {
        vertexParts.push(hierarchyResult.parentVertexData);
      }
      indexParts.push(clusterResult.indexData);
      if (hierarchyResult.parentIndexData.length > 0) {
        indexParts.push(hierarchyResult.parentIndexData);
      }

      // The root cluster's bounding sphere is already in world space since
      // geometry was baked.  Use it directly as the instance world bounds.
      const rootCluster = hierarchyResult.clusters[hierarchyResult.rootIndex - globalClusterOffset];
      const bs = rootCluster ? rootCluster.boundingSphere : new Float32Array([0, 0, 0, 1]);

      allInstances.push({
        // Geometry is pre-transformed; pass identity so any shader that reads
        // the transform doesn't accidentally double-transform.
        transform: mat4Identity(),
        meshId: mi,
        clusterOffset: globalClusterOffset,
        clusterCount: hierarchyResult.clusters.length,
        rootCluster: hierarchyResult.rootIndex,
        worldBounds: new Float32Array([bs[0], bs[1], bs[2], bs[3]]),
      });

      // Track per-mesh totals for the MeshDescriptor.
      if (meshRootCluster === -1) meshRootCluster = hierarchyResult.rootIndex;
      meshClusterCount += hierarchyResult.clusters.length;
      meshTotalVerts += clusterResult.mesh.totalVertices;
      meshTotalIndices += clusterResult.mesh.totalIndices;
      meshTotalTriangles += clusterResult.mesh.totalTriangles;
      for (const pid of clusterResult.mesh.pageIds) meshPageIds.push(pid);

      globalClusterOffset += hierarchyResult.clusters.length;
      globalVertexFloatOffset += clusterResult.vertexData.length +
        (hierarchyResult.parentVertexData.length ?? 0);
      globalIndexOffset += clusterResult.indexData.length +
        (hierarchyResult.parentIndexData.length ?? 0);
    }

    allMeshes.push({
      id: mi,
      name: entry.name,
      clusterOffset: meshClusterStart,
      clusterCount: meshClusterCount,
      rootCluster: meshRootCluster,
      pageIds: meshPageIds,
      totalVertices: meshTotalVerts,
      totalIndices: meshTotalIndices,
      totalTriangles: meshTotalTriangles,
    });
  }

  // Concatenate all vertex/index data
  let totalVerts = 0, totalIdx = 0;
  for (const v of vertexParts) totalVerts += v.length;
  for (const ix of indexParts) totalIdx += ix.length;

  const vertexData = new Float32Array(totalVerts);
  const indexData = new Uint32Array(totalIdx);
  let vo = 0, io = 0;
  for (const v of vertexParts) { vertexData.set(v, vo); vo += v.length; }
  for (const ix of indexParts) { indexData.set(ix, io); io += ix.length; }

  return {
    clusters: allClusters,
    pages: allPages,
    instances: allInstances,
    meshes: allMeshes,
    vertexData,
    indexData,
  };
}
