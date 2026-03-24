import { describe, it, expect } from 'vitest';
import {
  INSTANCE_CULL_SHADER,
  CLUSTER_LOD_SHADER,
  BUILD_INDIRECT_SHADER,
  VERTEX_PULL_SHADER,
  WIREFRAME_SHADER,
  HZB_SHADER,
  FULLSCREEN_QUAD_SHADER,
  RESET_COUNTER_SHADER,
} from '../shaders';

/**
 * Shader validation tests.
 * Since we can't compile WGSL without a GPU context, we validate
 * structural properties of each shader string.
 */

function hasEntryPoint(code: string, name: string, type: 'compute' | 'vertex' | 'fragment'): boolean {
  if (type === 'compute') {
    return code.includes(`@compute`) && code.includes(`fn ${name}`);
  }
  if (type === 'vertex') {
    return code.includes(`@vertex`) && code.includes(`fn ${name}`);
  }
  if (type === 'fragment') {
    return code.includes(`@fragment`) && code.includes(`fn ${name}`);
  }
  return false;
}

function hasBinding(code: string, group: number, binding: number): boolean {
  return code.includes(`@group(${group}) @binding(${binding})`);
}

function countBindings(code: string, group: number): number {
  const regex = new RegExp(`@group\\(${group}\\) @binding\\(\\d+\\)`, 'g');
  return (code.match(regex) || []).length;
}

describe('INSTANCE_CULL_SHADER', () => {
  it('has a compute entry point', () => {
    expect(hasEntryPoint(INSTANCE_CULL_SHADER, 'main', 'compute')).toBe(true);
  });

  it('has uniform, instance, visible output, and counter bindings', () => {
    expect(hasBinding(INSTANCE_CULL_SHADER, 0, 0)).toBe(true); // uniforms
    expect(hasBinding(INSTANCE_CULL_SHADER, 0, 1)).toBe(true); // instances
    expect(hasBinding(INSTANCE_CULL_SHADER, 0, 2)).toBe(true); // visible output
    expect(hasBinding(INSTANCE_CULL_SHADER, 0, 3)).toBe(true); // counter
  });

  it('uses workgroup_size(64)', () => {
    expect(INSTANCE_CULL_SHADER).toContain('@workgroup_size(64)');
  });

  it('references frustum culling', () => {
    expect(INSTANCE_CULL_SHADER).toContain('sphereOutsideFrustum');
  });

  it('uses atomicAdd for output', () => {
    expect(INSTANCE_CULL_SHADER).toContain('atomicAdd');
  });
});

describe('CLUSTER_LOD_SHADER', () => {
  it('has a compute entry point', () => {
    expect(hasEntryPoint(CLUSTER_LOD_SHADER, 'main', 'compute')).toBe(true);
  });

  it('reads cluster data', () => {
    expect(CLUSTER_LOD_SHADER).toContain('readClusterBoundingSphere');
    expect(CLUSTER_LOD_SHADER).toContain('readClusterLodError');
    expect(CLUSTER_LOD_SHADER).toContain('readClusterChildCount');
    expect(CLUSTER_LOD_SHADER).toContain('readClusterParent');
  });

  it('computes projected screen error', () => {
    expect(CLUSTER_LOD_SHADER).toContain('projectedScreenError');
  });

  it('checks LOD threshold', () => {
    expect(CLUSTER_LOD_SHADER).toContain('lodErrorThreshold');
  });

  it('handles leaf and parent nodes differently', () => {
    expect(CLUSTER_LOD_SHADER).toContain('childCount == 0u');
    expect(CLUSTER_LOD_SHADER).toContain('parentIdx');
  });

  it('writes per-cluster visibility flags (not compacted list)', () => {
    expect(CLUSTER_LOD_SHADER).toContain('clusterVisibility');
    expect(CLUSTER_LOD_SHADER).toContain('clusterVisibility[clusterIdx] = 1u');
    expect(CLUSTER_LOD_SHADER).toContain('clusterVisibility[clusterIdx] = 0u');
  });
});

describe('BUILD_INDIRECT_SHADER', () => {
  it('has a compute entry point', () => {
    expect(hasEntryPoint(BUILD_INDIRECT_SHADER, 'main', 'compute')).toBe(true);
  });

  it('writes draw command fields', () => {
    expect(BUILD_INDIRECT_SHADER).toContain('indexCount');
    expect(BUILD_INDIRECT_SHADER).toContain('indexOffset');
    expect(BUILD_INDIRECT_SHADER).toContain('vertexOffset');
  });
});

describe('VERTEX_PULL_SHADER', () => {
  it('has vertex and fragment entry points', () => {
    expect(hasEntryPoint(VERTEX_PULL_SHADER, 'vs_main', 'vertex')).toBe(true);
    expect(hasEntryPoint(VERTEX_PULL_SHADER, 'fs_main', 'fragment')).toBe(true);
  });

  it('pulls vertices from storage buffers', () => {
    expect(VERTEX_PULL_SHADER).toContain('var<storage, read> vertices');
    expect(VERTEX_PULL_SHADER).toContain('var<storage, read> indices');
  });

  it('uses builtin vertex_index and instance_index', () => {
    expect(VERTEX_PULL_SHADER).toContain('@builtin(vertex_index)');
    expect(VERTEX_PULL_SHADER).toContain('@builtin(instance_index)');
  });

  it('passes position, normal, UV, and cluster ID to fragment', () => {
    expect(VERTEX_PULL_SHADER).toContain('worldPos');
    expect(VERTEX_PULL_SHADER).toContain('normal');
    expect(VERTEX_PULL_SHADER).toContain('uv');
    expect(VERTEX_PULL_SHADER).toContain('clusterId');
  });

  it('checks per-cluster visibility flag', () => {
    expect(VERTEX_PULL_SHADER).toContain('clusterVisibility');
    expect(VERTEX_PULL_SHADER).toContain('isVisible == 0u');
  });

  it('uses instanceIndex directly as cluster ID', () => {
    expect(VERTEX_PULL_SHADER).toContain('let clusterIdx = instanceIndex');
  });

  it('has debug LOD coloring', () => {
    expect(VERTEX_PULL_SHADER).toContain('debugLODColors');
  });

  it('has basic lighting', () => {
    expect(VERTEX_PULL_SHADER).toContain('lightDir');
    expect(VERTEX_PULL_SHADER).toContain('ndotl');
    expect(VERTEX_PULL_SHADER).toContain('ambient');
    expect(VERTEX_PULL_SHADER).toContain('diffuse');
  });
});

describe('WIREFRAME_SHADER', () => {
  it('has vertex and fragment entry points', () => {
    expect(hasEntryPoint(WIREFRAME_SHADER, 'vs_main', 'vertex')).toBe(true);
    expect(hasEntryPoint(WIREFRAME_SHADER, 'fs_main', 'fragment')).toBe(true);
  });

  it('applies depth bias to avoid z-fighting', () => {
    expect(WIREFRAME_SHADER).toContain('biasedPos');
  });
});

describe('HZB_SHADER', () => {
  it('has a compute entry point', () => {
    expect(hasEntryPoint(HZB_SHADER, 'main', 'compute')).toBe(true);
  });

  it('uses 8x8 workgroup', () => {
    expect(HZB_SHADER).toContain('@workgroup_size(8, 8)');
  });

  it('reads input and writes output textures', () => {
    expect(HZB_SHADER).toContain('textureLoad');
    expect(HZB_SHADER).toContain('textureStore');
  });

  it('takes max depth of 2x2 block', () => {
    expect(HZB_SHADER).toContain('maxDepth');
  });
});

describe('RESET_COUNTER_SHADER', () => {
  it('has a compute entry point', () => {
    expect(hasEntryPoint(RESET_COUNTER_SHADER, 'main', 'compute')).toBe(true);
  });

  it('uses atomicStore', () => {
    expect(RESET_COUNTER_SHADER).toContain('atomicStore');
  });
});

describe('FULLSCREEN_QUAD_SHADER', () => {
  it('has vertex and fragment entry points', () => {
    expect(hasEntryPoint(FULLSCREEN_QUAD_SHADER, 'vs_main', 'vertex')).toBe(true);
    expect(hasEntryPoint(FULLSCREEN_QUAD_SHADER, 'fs_main', 'fragment')).toBe(true);
  });

  it('generates fullscreen triangle from vertex index', () => {
    expect(FULLSCREEN_QUAD_SHADER).toContain('@builtin(vertex_index)');
  });
});
