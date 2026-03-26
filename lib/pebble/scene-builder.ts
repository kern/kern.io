/**
 * Scene builder: takes raw meshes + instance placements,
 * runs the offline pipeline (clusterize → hierarchy → pages),
 * and produces a BuiltScene ready for GPU upload.
 */

import type { RawMesh } from './cluster-builder';
import type { BuiltScene, Instance, Cluster, Page, MeshDescriptor } from './types';
import { buildClusters } from './cluster-builder';
import { buildHierarchy } from './hierarchy-builder';
import { mat4Identity, mat4TransformPoint, Mat4, Vec3, v3len, v3sub } from './math';

export interface SceneMeshEntry {
  name: string;
  raw: RawMesh;
  instances: Mat4[]; // list of transforms
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

    // Step 1: clusterize
    const clusterResult = buildClusters(
      entry.raw, mi, entry.name,
      globalClusterOffset, globalVertexFloatOffset, globalIndexOffset,
    );

    // Step 2: build hierarchy
    const hierarchyResult = buildHierarchy(
      clusterResult.clusters,
      clusterResult.pages,
      clusterResult.vertexData,
      clusterResult.indexData,
      globalClusterOffset,
      globalVertexFloatOffset,
      globalIndexOffset,
    );

    // Update mesh descriptor
    clusterResult.mesh.rootCluster = hierarchyResult.rootIndex;
    clusterResult.mesh.clusterCount = hierarchyResult.clusters.length;

    // Accumulate
    allClusters.push(...hierarchyResult.clusters);
    allPages.push(...hierarchyResult.pages);
    allMeshes.push(clusterResult.mesh);
    vertexParts.push(clusterResult.vertexData);
    if (hierarchyResult.parentVertexData.length > 0) {
      vertexParts.push(hierarchyResult.parentVertexData);
    }
    indexParts.push(clusterResult.indexData);
    if (hierarchyResult.parentIndexData.length > 0) {
      indexParts.push(hierarchyResult.parentIndexData);
    }

    // Create instances
    for (const transform of entry.instances) {
      // Compute world bounds by transforming the mesh's root bounding sphere
      const rootCluster = hierarchyResult.clusters[hierarchyResult.rootIndex - globalClusterOffset];
      const bs = rootCluster ? rootCluster.boundingSphere : new Float32Array([0, 0, 0, 1]);
      const center: Vec3 = [bs[0], bs[1], bs[2]];
      const worldCenter = mat4TransformPoint(transform, center);
      // Approximate world radius (uniform scale assumption)
      const testPoint: Vec3 = [bs[0] + bs[3], bs[1], bs[2]];
      const worldTestPoint = mat4TransformPoint(transform, testPoint);
      const worldRadius = v3len(v3sub(worldTestPoint, worldCenter));

      allInstances.push({
        transform,
        meshId: mi,
        clusterOffset: globalClusterOffset,
        clusterCount: hierarchyResult.clusters.length,
        rootCluster: hierarchyResult.rootIndex,
        worldBounds: new Float32Array([worldCenter[0], worldCenter[1], worldCenter[2], worldRadius]),
      });
    }

    globalClusterOffset += hierarchyResult.clusters.length;
    globalVertexFloatOffset += clusterResult.vertexData.length +
      (hierarchyResult.parentVertexData ? hierarchyResult.parentVertexData.length : 0);
    globalIndexOffset += clusterResult.indexData.length +
      (hierarchyResult.parentIndexData ? hierarchyResult.parentIndexData.length : 0);
  }

  // Concatenate all vertex/index data
  let totalVerts = 0, totalIdx = 0;
  for (const v of vertexParts) totalVerts += v.length;
  for (const i of indexParts) totalIdx += i.length;

  const vertexData = new Float32Array(totalVerts);
  const indexData = new Uint32Array(totalIdx);
  let vo = 0, io = 0;
  for (const v of vertexParts) { vertexData.set(v, vo); vo += v.length; }
  for (const i of indexParts) { indexData.set(i, io); io += i.length; }

  return {
    clusters: allClusters,
    pages: allPages,
    instances: allInstances,
    meshes: allMeshes,
    vertexData,
    indexData,
  };
}
