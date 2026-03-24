import { describe, it, expect } from 'vitest';
import { buildScene } from '../scene-builder';
import { generateSphere, generateTorus, generateTerrain } from '../mesh-generator';
import { mat4Identity, mat4Translation } from '../math';

describe('buildScene', () => {
  it('builds a single-mesh scene', () => {
    const scene = buildScene([{
      name: 'sphere',
      raw: generateSphere(1, 8, 16),
      instances: [mat4Identity()],
    }]);

    expect(scene.clusters.length).toBeGreaterThan(0);
    expect(scene.pages.length).toBeGreaterThan(0);
    expect(scene.instances.length).toBe(1);
    expect(scene.meshes.length).toBe(1);
    expect(scene.vertexData.length).toBeGreaterThan(0);
    expect(scene.indexData.length).toBeGreaterThan(0);
  });

  it('builds a multi-mesh scene', () => {
    const scene = buildScene([
      {
        name: 'sphere',
        raw: generateSphere(1, 8, 16),
        instances: [mat4Identity(), mat4Translation(5, 0, 0)],
      },
      {
        name: 'torus',
        raw: generateTorus(2, 0.5, 16, 8),
        instances: [mat4Translation(0, 0, 5)],
      },
    ]);

    expect(scene.instances.length).toBe(3);
    expect(scene.meshes.length).toBe(2);
    // Clusters from both meshes
    expect(scene.clusters.length).toBeGreaterThan(5);
  });

  it('instances have valid world bounds', () => {
    const scene = buildScene([{
      name: 'sphere',
      raw: generateSphere(2, 8, 16),
      instances: [mat4Translation(10, 0, 0)],
    }]);

    const inst = scene.instances[0];
    // World bounds center should be near (10, 0, 0)
    expect(inst.worldBounds[0]).toBeCloseTo(10, 0);
    expect(inst.worldBounds[3]).toBeGreaterThan(0); // positive radius
  });

  it('all cluster indices are consistent', () => {
    const scene = buildScene([
      {
        name: 'terrain',
        raw: generateTerrain(10, 10, 16, 16),
        instances: [mat4Identity()],
      },
    ]);

    for (const cluster of scene.clusters) {
      expect(cluster.vertexCount).toBeGreaterThan(0);
      expect(cluster.indexCount).toBeGreaterThan(0);
      expect(cluster.indexCount % 3).toBe(0);
      expect(cluster.pageId).toBeGreaterThanOrEqual(0);
    }
  });

  it('handles large scene with many instances', () => {
    const instances = [];
    for (let i = 0; i < 10; i++) {
      instances.push(mat4Translation(i * 3, 0, 0));
    }

    const scene = buildScene([{
      name: 'sphere',
      raw: generateSphere(1, 8, 16),
      instances,
    }]);

    expect(scene.instances.length).toBe(10);
    // All instances share the same mesh clusters
    for (const inst of scene.instances) {
      expect(inst.meshId).toBe(0);
      expect(inst.clusterOffset).toBe(0);
    }
  });
});
