/**
 * Core data structures for the Pebble virtualized geometry renderer.
 *
 * Mental model:
 *   Offline: mesh → clusters → hierarchy (DAG) → pages
 *   Runtime: GPU traverses hierarchy, selects LOD, emits indirect draws
 */

// ─── Cluster ────────────────────────────────────────────────────────────────

/** A small batch of triangles (~64-128) that is the atomic unit of visibility. */
export interface Cluster {
  /** Bounding sphere: [cx, cy, cz, radius] */
  boundingSphere: Float32Array; // 4
  /** Normal cone for backface culling: [nx, ny, nz, cos(halfAngle)] */
  normalCone: Float32Array; // 4
  /** Conservative geometric error for this LOD node */
  lodError: number;
  /** Index into the cluster table for the parent (-1 = root) */
  parentIndex: number;
  /** First child index in clusterTable (-1 = leaf) */
  childOffset: number;
  /** Number of children */
  childCount: number;
  /** Byte offset into the vertex page buffer */
  vertexOffset: number;
  /** Number of vertices in this cluster */
  vertexCount: number;
  /** Byte offset into the index page buffer */
  indexOffset: number;
  /** Number of indices (triangleCount * 3) */
  indexCount: number;
  /** Material slot */
  materialId: number;
  /** Which page this cluster's geometry lives in */
  pageId: number;
  /** LOD level (0 = finest) */
  lodLevel: number;
}

// ─── Page ───────────────────────────────────────────────────────────────────

/** A page packs geometry for several clusters; it is the unit of streaming. */
export interface Page {
  id: number;
  /** Byte offset into the master vertex buffer */
  vertexBufferOffset: number;
  /** Byte size of vertex data */
  vertexBufferSize: number;
  /** Byte offset into the master index buffer */
  indexBufferOffset: number;
  /** Byte size of index data */
  indexBufferSize: number;
  /** Cluster indices that belong to this page */
  clusterIds: number[];
}

// ─── Instance ───────────────────────────────────────────────────────────────

/** A placed instance of a mesh in the scene. */
export interface Instance {
  /** 4x4 column-major transform */
  transform: Float32Array; // 16
  /** Index into the mesh table */
  meshId: number;
  /** First cluster index for this mesh in clusterTable */
  clusterOffset: number;
  /** Total cluster count for this mesh */
  clusterCount: number;
  /** Index of the root cluster (coarsest LOD) */
  rootCluster: number;
  /** Bounding sphere in world space */
  worldBounds: Float32Array; // 4
}

// ─── Mesh descriptor ────────────────────────────────────────────────────────

export interface MeshDescriptor {
  id: number;
  name: string;
  clusterOffset: number;
  clusterCount: number;
  rootCluster: number;
  pageIds: number[];
  totalVertices: number;
  totalIndices: number;
  totalTriangles: number;
}

// ─── GPU-side structs (flat, tightly packed for storage buffers) ─────────────

/**
 * GPU cluster record — 64 bytes, 16 × u32/f32.
 *
 * Layout (all little-endian):
 *   [0..3]   boundingSphere  (f32×4)
 *   [4..7]   normalCone      (f32×4)
 *   [8]      lodError        (f32)
 *   [9]      parentIndex     (u32)
 *   [10]     childOffset     (u32)
 *   [11]     childCount      (u32)
 *   [12]     vertexOffset    (u32)
 *   [13]     vertexCount     (u32)
 *   [14]     indexOffset     (u32)
 *   [15]     indexCount      (u32)
 */
export { GPU_CLUSTER_BYTES as GPU_CLUSTER_SIZE, GPU_CLUSTER_U32_STRIDE as GPU_CLUSTER_STRIDE } from './constants';

/**
 * GPU instance record — 80 bytes, 20 × f32/u32.
 *
 * Layout:
 *   [0..15]  transform       (f32×16, column-major mat4)
 *   [16..19] worldBounds     (f32×4)
 *   [20]     clusterOffset   (u32)
 *   [21]     clusterCount    (u32)
 *   [22]     rootCluster     (u32)
 *   [23]     meshId          (u32)
 */
export { GPU_INSTANCE_BYTES as GPU_INSTANCE_SIZE, GPU_INSTANCE_U32_STRIDE as GPU_INSTANCE_STRIDE } from './constants';

// ─── Scene ──────────────────────────────────────────────────────────────────

export interface BuiltScene {
  clusters: Cluster[];
  pages: Page[];
  instances: Instance[];
  meshes: MeshDescriptor[];
  /** Packed vertex buffer (position xyz + normal xyz + uv xy = 8 floats per vert) */
  vertexData: Float32Array;
  /** Packed index buffer */
  indexData: Uint32Array;
}

// ─── Camera / Render settings ───────────────────────────────────────────────

export interface Camera {
  position: Float32Array; // 3
  target: Float32Array; // 3
  up: Float32Array; // 3
  fovY: number; // radians
  aspect: number;
  near: number;
  far: number;
}

export interface RenderSettings {
  lodErrorThreshold: number; // pixel threshold for LOD switch
  freezeCulling: boolean;
  showWireframe: boolean;
  enableFrustumCulling: boolean;
  enableOcclusionCulling: boolean;
  enableBackfaceCulling: boolean;
  debugLODColors: boolean;
  maxTrianglesPerFrame: number;
}

import { DEFAULT_LOD_ERROR_THRESHOLD, DEFAULT_MAX_TRIANGLES_PER_FRAME } from './constants';

export const DEFAULT_RENDER_SETTINGS: RenderSettings = {
  lodErrorThreshold: DEFAULT_LOD_ERROR_THRESHOLD,
  freezeCulling: false,
  showWireframe: false,
  enableFrustumCulling: true,
  enableOcclusionCulling: false,
  enableBackfaceCulling: true,
  debugLODColors: true,
  maxTrianglesPerFrame: DEFAULT_MAX_TRIANGLES_PER_FRAME,
};

// ─── Stats ──────────────────────────────────────────────────────────────────

export interface FrameStats {
  fps: number;
  frameTimeMs: number;
  totalClusters: number;
  visibleClusters: number;
  totalTriangles: number;
  renderedTriangles: number;
  totalInstances: number;
  visibleInstances: number;
  residentPages: number;
  totalPages: number;
  gpuTimeMs: number;
}
