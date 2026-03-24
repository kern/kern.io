/**
 * All WGSL shaders for the Pebble renderer.
 *
 * Pass pipeline:
 *   1. Instance culling (compute)
 *   2. Cluster traversal + LOD selection (compute)
 *   3. Cluster compaction + indirect args (compute)
 *   4. Main draw (vertex pulling render)
 *   5. HZB generation (compute)
 *   6. Debug wireframe overlay (render)
 */

// ─── Shared struct definitions ──────────────────────────────────────────────

const SHARED_STRUCTS = /* wgsl */ `
struct ClusterData {
  // boundingSphere: vec4f at [0..3]
  // normalCone: vec4f at [4..7]
  // lodError: f32 at [8]
  // parentIndex: u32 at [9]
  // childOffset: u32 at [10]
  // childCount: u32 at [11]
  // vertexOffset: u32 at [12]
  // vertexCount: u32 at [13]
  // indexOffset: u32 at [14]
  // indexCount: u32 at [15]
  data: array<u32>,
}

struct InstanceData {
  // transform: mat4x4f at [0..15]
  // worldBounds: vec4f at [16..19]
  // clusterOffset: u32 at [20]
  // clusterCount: u32 at [21]
  // rootCluster: u32 at [22]
  // meshId: u32 at [23]
  data: array<u32>,
}

struct Uniforms {
  viewProj: mat4x4f,
  view: mat4x4f,
  proj: mat4x4f,
  cameraPos: vec4f,
  frustumPlanes: array<vec4f, 6>,
  screenHeight: f32,
  fovY: f32,
  lodErrorThreshold: f32,
  enableFrustumCulling: u32,
  enableBackfaceCulling: u32,
  enableOcclusionCulling: u32,
  debugLODColors: u32,
  totalClusters: u32,
  totalInstances: u32,
  _pad0: u32,
  _pad1: u32,
}

struct IndirectDraw {
  vertexCount: u32,
  instanceCount: u32,
  firstVertex: u32,
  firstInstance: u32,
}

struct DrawCommand {
  indexCount: u32,
  instanceCount: u32,
  firstIndex: u32,
  baseVertex: i32,
  firstInstance: u32,
}

fn readClusterBoundingSphere(clusterIdx: u32) -> vec4f {
  let base = clusterIdx * 16u;
  return vec4f(
    bitcast<f32>(clusters.data[base + 0u]),
    bitcast<f32>(clusters.data[base + 1u]),
    bitcast<f32>(clusters.data[base + 2u]),
    bitcast<f32>(clusters.data[base + 3u])
  );
}

fn readClusterNormalCone(clusterIdx: u32) -> vec4f {
  let base = clusterIdx * 16u;
  return vec4f(
    bitcast<f32>(clusters.data[base + 4u]),
    bitcast<f32>(clusters.data[base + 5u]),
    bitcast<f32>(clusters.data[base + 6u]),
    bitcast<f32>(clusters.data[base + 7u])
  );
}

fn readClusterLodError(clusterIdx: u32) -> f32 {
  return bitcast<f32>(clusters.data[clusterIdx * 16u + 8u]);
}

fn readClusterParent(clusterIdx: u32) -> u32 {
  return clusters.data[clusterIdx * 16u + 9u];
}

fn readClusterChildOffset(clusterIdx: u32) -> u32 {
  return clusters.data[clusterIdx * 16u + 10u];
}

fn readClusterChildCount(clusterIdx: u32) -> u32 {
  return clusters.data[clusterIdx * 16u + 11u];
}

fn readClusterVertexOffset(clusterIdx: u32) -> u32 {
  return clusters.data[clusterIdx * 16u + 12u];
}

fn readClusterVertexCount(clusterIdx: u32) -> u32 {
  return clusters.data[clusterIdx * 16u + 13u];
}

fn readClusterIndexOffset(clusterIdx: u32) -> u32 {
  return clusters.data[clusterIdx * 16u + 14u];
}

fn readClusterIndexCount(clusterIdx: u32) -> u32 {
  return clusters.data[clusterIdx * 16u + 15u];
}

fn sphereOutsideFrustum(center: vec3f, radius: f32) -> bool {
  for (var i = 0u; i < 6u; i = i + 1u) {
    let plane = uniforms.frustumPlanes[i];
    let dist = dot(plane.xyz, center) + plane.w;
    if (dist < -radius) {
      return true;
    }
  }
  return false;
}

fn projectedScreenError(geometricError: f32, distance: f32) -> f32 {
  if (distance < 0.001) { return 1000000.0; }
  let projScale = uniforms.screenHeight / (2.0 * tan(uniforms.fovY * 0.5));
  return (geometricError * projScale) / distance;
}
`;

// ─── Pass 1: Instance Culling ───────────────────────────────────────────────

export const INSTANCE_CULL_SHADER = /* wgsl */ `
${SHARED_STRUCTS}

@group(0) @binding(0) var<uniform> uniforms: Uniforms;
@group(0) @binding(1) var<storage, read> instances: InstanceData;
@group(0) @binding(2) var<storage, read_write> visibleInstances: array<u32>;
@group(0) @binding(3) var<storage, read_write> visibleCount: array<atomic<u32>>;
@group(0) @binding(4) var<storage, read> clusters: ClusterData;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3u) {
  let instanceIdx = gid.x;
  if (instanceIdx >= uniforms.totalInstances) { return; }

  let base = instanceIdx * 24u;
  let bx = bitcast<f32>(instances.data[base + 16u]);
  let by = bitcast<f32>(instances.data[base + 17u]);
  let bz = bitcast<f32>(instances.data[base + 18u]);
  let br = bitcast<f32>(instances.data[base + 19u]);

  // Frustum cull
  if (uniforms.enableFrustumCulling != 0u) {
    if (sphereOutsideFrustum(vec3f(bx, by, bz), br)) {
      return;
    }
  }

  // Add to visible list
  let idx = atomicAdd(&visibleCount[0], 1u);
  visibleInstances[idx] = instanceIdx;
}
`;

// ─── Pass 2: Cluster Traversal + LOD Selection ─────────────────────────────

export const CLUSTER_LOD_SHADER = /* wgsl */ `
${SHARED_STRUCTS}

@group(0) @binding(0) var<uniform> uniforms: Uniforms;
@group(0) @binding(1) var<storage, read> clusters: ClusterData;
@group(0) @binding(2) var<storage, read_write> clusterVisibility: array<u32>;
@group(0) @binding(3) var<storage, read_write> visibleClusterCount: array<atomic<u32>>;

// Per-cluster visibility flag approach:
// Write 1 (visible) or 0 (hidden) at clusterVisibility[clusterIdx].
// The vertex shader reads this flag and emits degenerate triangles for hidden clusters.
// This avoids the indirection mismatch that causes flashing.
@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3u) {
  let clusterIdx = gid.x;
  if (clusterIdx >= uniforms.totalClusters) { return; }

  // Default: not visible
  clusterVisibility[clusterIdx] = 0u;

  let bs = readClusterBoundingSphere(clusterIdx);
  let center = bs.xyz;
  let radius = bs.w;

  // Frustum cull
  if (uniforms.enableFrustumCulling != 0u) {
    if (sphereOutsideFrustum(center, radius)) {
      return;
    }
  }

  // Backface cone cull
  if (uniforms.enableBackfaceCulling != 0u) {
    let nc = readClusterNormalCone(clusterIdx);
    let coneDir = nc.xyz;
    let coneCos = nc.w;
    if (length(coneDir) > 0.01 && coneCos > 0.0) {
      let viewDir = normalize(center - uniforms.cameraPos.xyz);
      let d = dot(viewDir, coneDir);
      // All normals backfacing when dot(viewDir, coneDir) > sin(halfAngle)
      // sin(halfAngle) = sqrt(1 - coneCos²)
      if (d > sqrt(1.0 - coneCos * coneCos)) {
        return;
      }
    }
  }

  // LOD selection: check if this cluster should be rendered
  let lodError = readClusterLodError(clusterIdx);
  let dist = length(center - uniforms.cameraPos.xyz);
  let screenErr = projectedScreenError(lodError, dist);
  let childCount = readClusterChildCount(clusterIdx);
  let parentIdx = readClusterParent(clusterIdx);

  // A cluster is visible if:
  // 1. It's a leaf (no children) OR its screen error is below threshold
  // 2. AND its parent's error is above threshold (parent would want to refine)
  var shouldRender = false;

  if (childCount == 0u) {
    // Leaf cluster: always a candidate
    shouldRender = true;
  } else if (screenErr < uniforms.lodErrorThreshold) {
    // This node's error is small enough — use it instead of refining to children
    shouldRender = true;
  }

  // But only render if parent WOULD refine to us
  // (i.e., parent's error is above threshold)
  // Use the child's distance for parent error projection so both are evaluated
  // at the same distance — avoids incorrectly suppressing children when the
  // parent bounding sphere center is far from the child cluster.
  if (shouldRender && parentIdx != 0xFFFFFFFFu && parentIdx < uniforms.totalClusters) {
    let parentError = readClusterLodError(parentIdx);
    let parentScreenErr = projectedScreenError(parentError, dist);
    if (parentScreenErr < uniforms.lodErrorThreshold) {
      // Parent error is also small — parent will render itself, skip this child
      shouldRender = false;
    }
  }

  if (shouldRender) {
    clusterVisibility[clusterIdx] = 1u;
    atomicAdd(&visibleClusterCount[0], 1u);
  }
}
`;

// ─── Pass 3: Build Indirect Draw Args ───────────────────────────────────────

export const BUILD_INDIRECT_SHADER = /* wgsl */ `
struct ClusterRecord {
  data: array<u32>,
}

@group(0) @binding(0) var<storage, read> visibleClusters: array<u32>;
@group(0) @binding(1) var<storage, read> visibleClusterCount: array<u32>;
@group(0) @binding(2) var<storage, read> clusters: ClusterRecord;
@group(0) @binding(3) var<storage, read_write> drawCommands: array<u32>;
@group(0) @binding(4) var<storage, read_write> drawCount: array<atomic<u32>>;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3u) {
  let idx = gid.x;
  if (idx >= visibleClusterCount[0]) { return; }

  let clusterIdx = visibleClusters[idx];
  let base = clusterIdx * 16u;
  let indexCount = clusters.data[base + 15u];
  let indexOffset = clusters.data[base + 14u];
  let vertexOffset = clusters.data[base + 12u];

  // Write indexed indirect draw command
  // Each command is 5 u32s: indexCount, instanceCount, firstIndex, baseVertex, firstInstance
  let cmdBase = idx * 5u;
  drawCommands[cmdBase + 0u] = indexCount;      // indexCount
  drawCommands[cmdBase + 1u] = 1u;              // instanceCount
  drawCommands[cmdBase + 2u] = indexOffset;      // firstIndex
  drawCommands[cmdBase + 3u] = vertexOffset / 8u; // baseVertex (vertex stride = 8 floats)
  drawCommands[cmdBase + 4u] = clusterIdx;       // firstInstance (pass cluster ID)

  atomicAdd(&drawCount[0], 1u);
}
`;

// ─── Pass 4: Main Vertex-Pulling Render ─────────────────────────────────────

export const VERTEX_PULL_SHADER = /* wgsl */ `
struct Uniforms {
  viewProj: mat4x4f,
  view: mat4x4f,
  proj: mat4x4f,
  cameraPos: vec4f,
  frustumPlanes: array<vec4f, 6>,
  screenHeight: f32,
  fovY: f32,
  lodErrorThreshold: f32,
  enableFrustumCulling: u32,
  enableBackfaceCulling: u32,
  enableOcclusionCulling: u32,
  debugLODColors: u32,
  totalClusters: u32,
  totalInstances: u32,
  _pad0: u32,
  _pad1: u32,
}

struct VertexOutput {
  @builtin(position) position: vec4f,
  @location(0) worldPos: vec3f,
  @location(1) normal: vec3f,
  @location(2) uv: vec2f,
  @location(3) @interpolate(flat) clusterId: u32,
  @location(4) @interpolate(flat) lodLevel: u32,
}

@group(0) @binding(0) var<uniform> uniforms: Uniforms;
@group(0) @binding(1) var<storage, read> vertices: array<f32>;
@group(0) @binding(2) var<storage, read> indices: array<u32>;
@group(0) @binding(3) var<storage, read> clusterData: array<u32>;
@group(0) @binding(4) var<storage, read> clusterVisibility: array<u32>;

// Vertex pulling: instance_index = cluster ID directly.
// The compute pass writes a per-cluster visibility flag (1=visible, 0=hidden).
// Hidden clusters get degenerate triangles (all vertices at origin behind clip).

@vertex
fn vs_main(
  @builtin(vertex_index) vertexIndex: u32,
  @builtin(instance_index) instanceIndex: u32,
) -> VertexOutput {
  var out: VertexOutput;

  // instanceIndex IS the cluster ID — no indirection
  let clusterIdx = instanceIndex;
  let clusterBase = clusterIdx * 16u;

  // Check visibility flag from compute pass
  let isVisible = clusterVisibility[clusterIdx];
  if (isVisible == 0u) {
    // Emit degenerate triangle — all verts at clip origin, will be clipped
    out.position = vec4f(0.0, 0.0, 2.0, 1.0); // behind far plane
    out.worldPos = vec3f(0.0);
    out.normal = vec3f(0.0, 1.0, 0.0);
    out.uv = vec2f(0.0);
    out.clusterId = clusterIdx;
    out.lodLevel = 0u;
    return out;
  }

  let vertexOffset = clusterData[clusterBase + 12u];
  let indexOffset = clusterData[clusterBase + 14u];

  // Read the actual vertex index from the index buffer
  let actualIndex = indices[indexOffset + vertexIndex];

  // Read vertex data: 8 floats per vertex (pos3 + normal3 + uv2)
  let vBase = vertexOffset + actualIndex * 8u;
  let pos = vec3f(vertices[vBase], vertices[vBase + 1u], vertices[vBase + 2u]);
  let normal = vec3f(vertices[vBase + 3u], vertices[vBase + 4u], vertices[vBase + 5u]);
  let uv = vec2f(vertices[vBase + 6u], vertices[vBase + 7u]);

  out.position = uniforms.viewProj * vec4f(pos, 1.0);
  out.worldPos = pos;
  out.normal = normal;
  out.uv = uv;
  out.clusterId = clusterIdx;

  // Derive LOD level from child count
  let childCount = clusterData[clusterBase + 11u];
  if (childCount == 0u) {
    out.lodLevel = 0u;
  } else {
    out.lodLevel = 1u + childCount;
  }

  return out;
}

@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4f {
  let normal = normalize(in.normal);

  // Simple directional lighting
  let lightDir = normalize(vec3f(0.5, 1.0, 0.3));
  let ndotl = max(dot(normal, lightDir), 0.0);
  let ambient = 0.15;
  let diffuse = ndotl * 0.85;

  var baseColor: vec3f;

  if (uniforms.debugLODColors != 0u) {
    // Color by cluster ID for visualization
    let hash = in.clusterId * 2654435761u;
    let r = f32((hash >> 0u) & 0xFFu) / 255.0;
    let g = f32((hash >> 8u) & 0xFFu) / 255.0;
    let b = f32((hash >> 16u) & 0xFFu) / 255.0;
    baseColor = vec3f(r * 0.7 + 0.3, g * 0.7 + 0.3, b * 0.7 + 0.3);
  } else {
    baseColor = vec3f(0.8, 0.75, 0.7); // neutral warm gray
  }

  let color = baseColor * (ambient + diffuse);

  // Simple fog
  let dist = length(in.worldPos - uniforms.cameraPos.xyz);
  let fog = clamp(dist / 100.0, 0.0, 0.8);
  let fogColor = vec3f(0.6, 0.65, 0.75);
  let finalColor = mix(color, fogColor, fog);

  return vec4f(finalColor, 1.0);
}
`;

// ─── Wireframe overlay ──────────────────────────────────────────────────────

export const WIREFRAME_SHADER = /* wgsl */ `
struct Uniforms {
  viewProj: mat4x4f,
  view: mat4x4f,
  proj: mat4x4f,
  cameraPos: vec4f,
  frustumPlanes: array<vec4f, 6>,
  screenHeight: f32,
  fovY: f32,
  lodErrorThreshold: f32,
  enableFrustumCulling: u32,
  enableBackfaceCulling: u32,
  enableOcclusionCulling: u32,
  debugLODColors: u32,
  totalClusters: u32,
  totalInstances: u32,
  _pad0: u32,
  _pad1: u32,
}

@group(0) @binding(0) var<uniform> uniforms: Uniforms;
@group(0) @binding(1) var<storage, read> vertices: array<f32>;
@group(0) @binding(2) var<storage, read> indices: array<u32>;
@group(0) @binding(3) var<storage, read> clusterData: array<u32>;
@group(0) @binding(4) var<storage, read> clusterVisibility: array<u32>;

struct VertexOutput {
  @builtin(position) position: vec4f,
}

@vertex
fn vs_main(
  @builtin(vertex_index) vertexIndex: u32,
  @builtin(instance_index) instanceIndex: u32,
) -> VertexOutput {
  var out: VertexOutput;

  let clusterIdx = instanceIndex;

  // Check visibility flag
  if (clusterVisibility[clusterIdx] == 0u) {
    out.position = vec4f(0.0, 0.0, 2.0, 1.0);
    return out;
  }

  let clusterBase = clusterIdx * 16u;
  let vertexOffset = clusterData[clusterBase + 12u];
  let indexOffset = clusterData[clusterBase + 14u];
  let actualIndex = indices[indexOffset + vertexIndex];
  let vBase = vertexOffset + actualIndex * 8u;
  let pos = vec3f(vertices[vBase], vertices[vBase + 1u], vertices[vBase + 2u]);

  // Slight offset toward camera for depth bias
  let viewDir = normalize(uniforms.cameraPos.xyz - pos);
  let biasedPos = pos + viewDir * 0.002;
  out.position = uniforms.viewProj * vec4f(biasedPos, 1.0);

  return out;
}

@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4f {
  return vec4f(0.0, 0.0, 0.0, 1.0);
}
`;

// ─── HZB Generation ─────────────────────────────────────────────────────────

export const HZB_SHADER = /* wgsl */ `
@group(0) @binding(0) var inputTex: texture_2d<f32>;
@group(0) @binding(1) var outputTex: texture_storage_2d<r32float, write>;

@compute @workgroup_size(8, 8)
fn main(@builtin(global_invocation_id) gid: vec3u) {
  let outSize = textureDimensions(outputTex);
  if (gid.x >= outSize.x || gid.y >= outSize.y) { return; }

  let inCoord = vec2u(gid.x * 2u, gid.y * 2u);
  let inSize = textureDimensions(inputTex);

  // Take max depth of 2x2 block (conservative for occlusion)
  var maxDepth = 0.0;
  for (var dy = 0u; dy < 2u; dy = dy + 1u) {
    for (var dx = 0u; dx < 2u; dx = dx + 1u) {
      let coord = min(inCoord + vec2u(dx, dy), inSize - vec2u(1u, 1u));
      let d = textureLoad(inputTex, coord, 0).r;
      maxDepth = max(maxDepth, d);
    }
  }

  textureStore(outputTex, vec2i(gid.xy), vec4f(maxDepth, 0.0, 0.0, 1.0));
}
`;

// ─── Fullscreen quad (for depth blit / debug view) ──────────────────────────

export const FULLSCREEN_QUAD_SHADER = /* wgsl */ `
struct VertexOutput {
  @builtin(position) position: vec4f,
  @location(0) uv: vec2f,
}

@vertex
fn vs_main(@builtin(vertex_index) vi: u32) -> VertexOutput {
  var out: VertexOutput;
  // Fullscreen triangle
  let x = f32(i32(vi & 1u)) * 4.0 - 1.0;
  let y = f32(i32(vi >> 1u)) * 4.0 - 1.0;
  out.position = vec4f(x, y, 0.0, 1.0);
  out.uv = vec2f((x + 1.0) * 0.5, (1.0 - y) * 0.5);
  return out;
}

@group(0) @binding(0) var texSampler: sampler;
@group(0) @binding(1) var inputTex: texture_2d<f32>;

@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4f {
  let d = textureSample(inputTex, texSampler, in.uv).r;
  return vec4f(vec3f(d), 1.0);
}
`;

// ─── Counter reset (utility) ────────────────────────────────────────────────

export const RESET_COUNTER_SHADER = /* wgsl */ `
@group(0) @binding(0) var<storage, read_write> counter: array<atomic<u32>>;

@compute @workgroup_size(1)
fn main() {
  atomicStore(&counter[0], 0u);
}
`;
