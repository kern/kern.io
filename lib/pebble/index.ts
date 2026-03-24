/**
 * Public API for the Pebble WebGPU renderer.
 */

export { PebbleRenderer } from './renderer';
export { buildScene } from './scene-builder';
export { buildClusters } from './cluster-builder';
export { buildHierarchy } from './hierarchy-builder';
export { PageManager } from './page-manager';
export {
  generateSphere,
  generatePlane,
  generateTerrain,
  generateTorus,
  generateMassiveScene,
} from './mesh-generator';
export type {
  BuiltScene,
  Camera,
  RenderSettings,
  FrameStats,
  Cluster,
  Page,
  Instance,
  MeshDescriptor,
} from './types';
export { DEFAULT_RENDER_SETTINGS } from './types';
export {
  mat4Identity,
  mat4Translation,
  mat4Scale,
  mat4RotationY,
  mat4Multiply,
  mat4Perspective,
  mat4LookAt,
} from './math';
