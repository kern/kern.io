/**
 * All magic constants for the Pebble virtualized geometry renderer.
 * Centralized here so they can be tuned from one place.
 */

// ─── Cluster Builder ────────────────────────────────────────────────────────

/** Target number of triangles per cluster (ideal). */
export const TARGET_CLUSTER_TRIANGLES = 64;

/** Maximum triangles a single cluster may contain. */
export const MAX_CLUSTER_TRIANGLES = 128;

/** Number of clusters packed into a single streaming page. */
export const CLUSTERS_PER_PAGE = 16;

/** Floats per vertex in the packed vertex format (pos3 + normal3 + uv2). */
export const VERTEX_STRIDE_FLOATS = 8;

/**
 * Geometric error for leaf clusters, expressed as a fraction of the
 * cluster bounding-sphere radius. A more accurate metric would compare
 * against the original mesh surface, but radius * this factor is a
 * reasonable first-pass proxy.
 */
export const LEAF_ERROR_RADIUS_FRACTION = 0.1;

// ─── Hierarchy Builder ──────────────────────────────────────────────────────

/** Maximum children each internal (parent) node in the cluster DAG may have. */
export const MAX_CHILDREN_PER_PARENT = 4;

/** Minimum leaf-cluster count before we bother building a hierarchy. */
export const MIN_CLUSTERS_FOR_HIERARCHY = 4;

/**
 * When generating simplified geometry for a parent node, we keep every
 * Nth vertex from the merged children. Lower = better quality parents
 * at the cost of more geometry.
 */
export const PARENT_VERTEX_SUBSAMPLE_STEP = 4;

/**
 * Error scale factor: parent.lodError = max(childErrors) * this.
 * Must be > 1 so parent errors are always >= child errors.
 */
export const PARENT_ERROR_SCALE = 2.0;

// ─── Page Manager ───────────────────────────────────────────────────────────

/** Default maximum number of concurrently-resident pages. */
export const DEFAULT_MAX_RESIDENT_PAGES = 256;

/** Default per-frame upload budget in bytes (4 MB). */
export const DEFAULT_UPLOAD_BUDGET_BYTES = 4 * 1024 * 1024;

// ─── GPU Layout ─────────────────────────────────────────────────────────────

/** Bytes per GPU cluster record. */
export const GPU_CLUSTER_BYTES = 64;

/** u32 stride per GPU cluster record (GPU_CLUSTER_BYTES / 4). */
export const GPU_CLUSTER_U32_STRIDE = 16;

/** u32 stride per GPU instance record. */
export const GPU_INSTANCE_U32_STRIDE = 24;

/** Bytes per GPU instance record. */
export const GPU_INSTANCE_BYTES = GPU_INSTANCE_U32_STRIDE * 4;

/** Minimum GPU buffer size (WebGPU requires buffers >= 1 byte, we use 16 for alignment). */
export const MIN_BUFFER_SIZE = 16;

/** Size of the uniform buffer in bytes. */
export const UNIFORM_BUFFER_SIZE = 512;

// ─── Compute Shaders ────────────────────────────────────────────────────────

/** Workgroup size for 1D compute dispatches (instance cull, cluster LOD, etc.). */
export const COMPUTE_WORKGROUP_SIZE_1D = 64;

/** Workgroup size for 2D compute dispatches (HZB generation). */
export const COMPUTE_WORKGROUP_SIZE_2D = 8;

// ─── Renderer Defaults ──────────────────────────────────────────────────────

/** Sentinel value for "no parent" / "no children" in cluster links. */
export const INVALID_INDEX = 0xFFFFFFFF;

/** Fields per indirect draw command (indexCount, instanceCount, firstIndex, baseVertex, firstInstance). */
export const INDIRECT_DRAW_COMMAND_U32S = 5;

// ─── LOD Selection ──────────────────────────────────────────────────────────

/**
 * Default LOD error threshold in pixels. Clusters whose projected
 * screen-space error is below this value are accepted; otherwise
 * the system tries to refine to their children.
 */
export const DEFAULT_LOD_ERROR_THRESHOLD = 1.0;

// ─── Rendering ──────────────────────────────────────────────────────────────

/** Clear color for the main render pass (dark blue-gray). */
export const CLEAR_COLOR = { r: 0.15, g: 0.15, b: 0.2, a: 1.0 } as const;

/** Depth clear value (far plane = 1.0). */
export const DEPTH_CLEAR_VALUE = 1.0;

/** Wireframe depth bias: small offset toward camera to prevent z-fighting. */
export const WIREFRAME_DEPTH_BIAS = 0.002;

// ─── Camera Defaults ────────────────────────────────────────────────────────

/** Default camera orbit distance. */
export const DEFAULT_CAMERA_DISTANCE = 8;

/** Default camera azimuth (theta). */
export const DEFAULT_CAMERA_THETA = 0.5;

/** Default camera elevation (phi). */
export const DEFAULT_CAMERA_PHI = 0.4;

/** Min/max zoom distances. */
export const MIN_ZOOM_DISTANCE = 1;
export const MAX_ZOOM_DISTANCE = 200;

/** Mouse orbit sensitivity. */
export const ORBIT_SENSITIVITY = 0.005;

/** Mouse pan sensitivity multiplier (relative to distance). */
export const PAN_SENSITIVITY = 0.002;

/** Scroll zoom multiplier. */
export const ZOOM_SCROLL_FACTOR = 1.1;

// ─── FPS Counter ────────────────────────────────────────────────────────────

/** Interval (ms) between FPS counter updates. */
export const FPS_UPDATE_INTERVAL_MS = 1000;

// ─── Max limits ─────────────────────────────────────────────────────────────

/** Default cap on rendered triangles per frame. */
export const DEFAULT_MAX_TRIANGLES_PER_FRAME = 10_000_000;

/** Max storage buffer binding size requested from WebGPU device. */
export const MAX_STORAGE_BUFFER_BINDING_SIZE = 256 * 1024 * 1024;

/** Max buffer size requested from WebGPU device. */
export const MAX_BUFFER_SIZE = 256 * 1024 * 1024;
