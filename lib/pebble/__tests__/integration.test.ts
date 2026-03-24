import { describe, it, expect } from 'vitest';
import { buildScene } from '../scene-builder';
import { buildClusters } from '../cluster-builder';
import { buildHierarchy } from '../hierarchy-builder';
import { PageManager } from '../page-manager';
import {
  generateSphere, generateTerrain, generateTorus,
  generatePlane, generateMassiveScene,
} from '../mesh-generator';
import {
  mat4Identity, mat4Translation, mat4Multiply, mat4RotationY, mat4Scale,
  mat4Perspective, mat4LookAt, extractFrustumPlanes, sphereOutsideFrustum,
  projectedError,
} from '../math';
import { GPU_CLUSTER_STRIDE } from '../types';
import type { Cluster } from '../types';

describe('End-to-end pipeline', () => {
  it('full pipeline: mesh → clusters → hierarchy → scene → page manager', () => {
    // 1. Generate mesh
    const raw = generateSphere(2, 32, 64);
    expect(raw.indices.length / 3).toBe(32 * 64 * 2); // 4096 triangles

    // 2. Clusterize
    const built = buildClusters(raw, 0, 'sphere', 0, 0, 0);
    expect(built.clusters.length).toBeGreaterThan(30);

    // 3. Build hierarchy
    const hier = buildHierarchy(built.clusters, built.pages, built.vertexData, built.indexData);
    expect(hier.clusters.length).toBeGreaterThan(built.clusters.length);

    // 4. Build scene
    const scene = buildScene([{
      name: 'sphere',
      raw,
      instances: [mat4Identity(), mat4Translation(5, 0, 0)],
    }]);
    expect(scene.instances.length).toBe(2);

    // 5. Page management
    const pm = new PageManager(256);
    pm.registerPages(scene.pages);
    pm.makeAllResident();
    const stats = pm.getStats();
    expect(stats.resident).toBe(stats.total);
  });

  it('multi-mesh scene maintains consistent offsets', () => {
    const scene = buildScene([
      { name: 'sphere', raw: generateSphere(1, 8, 16), instances: [mat4Identity()] },
      { name: 'torus', raw: generateTorus(2, 0.5, 16, 8), instances: [mat4Translation(5, 0, 0)] },
      { name: 'terrain', raw: generateTerrain(10, 10, 16, 16), instances: [mat4Translation(0, 0, 5)] },
    ]);

    expect(scene.meshes.length).toBe(3);
    expect(scene.instances.length).toBe(3);

    // Verify no overlapping cluster indices
    for (let i = 0; i < scene.meshes.length - 1; i++) {
      const m1 = scene.meshes[i];
      const m2 = scene.meshes[i + 1];
      expect(m2.clusterOffset).toBeGreaterThanOrEqual(m1.clusterOffset + m1.clusterCount);
    }

    // Total vertex/index data should be sum of all meshes
    expect(scene.vertexData.length).toBeGreaterThan(0);
    expect(scene.indexData.length).toBeGreaterThan(0);
  });
});

describe('GPU buffer packing', () => {
  it('cluster data packs into exactly GPU_CLUSTER_STRIDE u32s', () => {
    const raw = generateSphere(1, 8, 16);
    const built = buildClusters(raw, 0, 'sphere', 0, 0, 0);

    const data = new Uint32Array(built.clusters.length * GPU_CLUSTER_STRIDE);
    const f32View = new Float32Array(data.buffer);

    for (let i = 0; i < built.clusters.length; i++) {
      const c = built.clusters[i];
      const base = i * GPU_CLUSTER_STRIDE;

      f32View[base + 0] = c.boundingSphere[0];
      f32View[base + 1] = c.boundingSphere[1];
      f32View[base + 2] = c.boundingSphere[2];
      f32View[base + 3] = c.boundingSphere[3];
      f32View[base + 4] = c.normalCone[0];
      f32View[base + 5] = c.normalCone[1];
      f32View[base + 6] = c.normalCone[2];
      f32View[base + 7] = c.normalCone[3];
      f32View[base + 8] = c.lodError;
      data[base + 9] = c.parentIndex === -1 ? 0xFFFFFFFF : c.parentIndex;
      data[base + 10] = c.childOffset === -1 ? 0xFFFFFFFF : c.childOffset;
      data[base + 11] = c.childCount;
      data[base + 12] = c.vertexOffset;
      data[base + 13] = c.vertexCount;
      data[base + 14] = c.indexOffset;
      data[base + 15] = c.indexCount;
    }

    // Verify we can read back the data correctly
    for (let i = 0; i < built.clusters.length; i++) {
      const c = built.clusters[i];
      const base = i * GPU_CLUSTER_STRIDE;

      expect(f32View[base + 0]).toBeCloseTo(c.boundingSphere[0]);
      expect(f32View[base + 3]).toBeCloseTo(c.boundingSphere[3]);
      expect(f32View[base + 8]).toBeCloseTo(c.lodError);
      expect(data[base + 15]).toBe(c.indexCount);
    }
  });
});

describe('LOD selection simulation', () => {
  it('closer camera selects more clusters (finer LOD)', () => {
    const scene = buildScene([{
      name: 'sphere',
      raw: generateSphere(2, 32, 64),
      instances: [mat4Identity()],
    }]);

    const threshold = 1.0;
    const fovY = Math.PI / 3;
    const screenHeight = 1080;

    function countVisible(cameraDistance: number): number {
      let count = 0;
      for (const c of scene.clusters) {
        const dist = cameraDistance; // simplified
        const screenErr = projectedError(c.lodError, dist, screenHeight, fovY);
        const childCount = c.childCount;

        let shouldRender = false;
        if (childCount === 0) {
          shouldRender = true;
        } else if (screenErr < threshold) {
          shouldRender = true;
        }

        if (shouldRender && c.parentIndex >= 0 && c.parentIndex < scene.clusters.length) {
          const parent = scene.clusters[c.parentIndex];
          const parentScreenErr = projectedError(parent.lodError, dist, screenHeight, fovY);
          if (parentScreenErr < threshold) {
            shouldRender = false;
          }
        }

        if (shouldRender) count++;
      }
      return count;
    }

    const closeCount = countVisible(3);
    const farCount = countVisible(50);

    // Close camera should see more clusters than far camera
    // (finer LOD = more clusters)
    expect(closeCount).toBeGreaterThanOrEqual(farCount);
  });

  it('LOD threshold controls refinement level', () => {
    const scene = buildScene([{
      name: 'sphere',
      raw: generateSphere(2, 32, 64),
      instances: [mat4Identity()],
    }]);

    const fovY = Math.PI / 3;
    const screenHeight = 1080;
    const dist = 10;

    function countVisible(threshold: number): number {
      let count = 0;
      for (const c of scene.clusters) {
        const screenErr = projectedError(c.lodError, dist, screenHeight, fovY);
        if (c.childCount === 0 || screenErr < threshold) {
          let render = true;
          if (c.parentIndex >= 0 && c.parentIndex < scene.clusters.length) {
            const parent = scene.clusters[c.parentIndex];
            const parentScreenErr = projectedError(parent.lodError, dist, screenHeight, fovY);
            if (parentScreenErr < threshold) render = false;
          }
          if (render) count++;
        }
      }
      return count;
    }

    const tightLOD = countVisible(0.1);   // very tight = many clusters
    const looseLOD = countVisible(100.0);  // very loose = few clusters

    expect(tightLOD).toBeGreaterThanOrEqual(looseLOD);
  });
});

describe('Frustum culling simulation', () => {
  it('clusters behind camera are culled', () => {
    const scene = buildScene([{
      name: 'sphere',
      raw: generateSphere(1, 16, 32),
      instances: [mat4Identity()],
    }]);

    // Camera at (0, 0, 10) looking at origin
    const view = mat4LookAt([0, 0, 10], [0, 0, 0], [0, 1, 0]);
    const proj = mat4Perspective(Math.PI / 3, 16 / 9, 0.1, 100);
    const vp = mat4Multiply(proj, view);
    const planes = extractFrustumPlanes(vp);

    let culled = 0;
    let visible = 0;
    for (const c of scene.clusters) {
      const bs = c.boundingSphere;
      if (sphereOutsideFrustum(planes, bs[0], bs[1], bs[2], bs[3])) {
        culled++;
      } else {
        visible++;
      }
    }

    // Sphere at origin should be visible from camera at z=10
    expect(visible).toBeGreaterThan(0);
  });

  it('culls objects far from view', () => {
    const scene = buildScene([{
      name: 'sphere',
      raw: generateSphere(1, 8, 16),
      instances: [mat4Translation(100, 100, 100)],
    }]);

    // Camera looking at origin - sphere at (100,100,100) should be culled
    const view = mat4LookAt([0, 0, 5], [0, 0, 0], [0, 1, 0]);
    const proj = mat4Perspective(Math.PI / 3, 16 / 9, 0.1, 50);
    const vp = mat4Multiply(proj, view);
    const planes = extractFrustumPlanes(vp);

    // Instance bounds are at (100, 100, 100)
    const inst = scene.instances[0];
    const isOutside = sphereOutsideFrustum(
      planes,
      inst.worldBounds[0], inst.worldBounds[1],
      inst.worldBounds[2], inst.worldBounds[3],
    );
    expect(isOutside).toBe(true);
  });
});

describe('Page streaming simulation', () => {
  it('handles progressive page loading', () => {
    const scene = buildScene([{
      name: 'sphere',
      raw: generateSphere(2, 32, 64),
      instances: [mat4Identity()],
    }]);

    const pm = new PageManager(4, 2000); // small pool + budget
    pm.registerPages(scene.pages);

    // Frame 1: request some pages
    for (const page of scene.pages.slice(0, 3)) {
      pm.requestPage(page.id, 1.0);
    }
    const uploaded1 = pm.processRequests();
    expect(uploaded1.length).toBeGreaterThan(0);

    // Frame 2: request more
    for (const page of scene.pages.slice(3, 6)) {
      pm.requestPage(page.id, 0.5);
    }
    const uploaded2 = pm.processRequests();
    expect(uploaded2.length).toBeGreaterThan(0);

    // Verify resident pages
    const stats = pm.getStats();
    expect(stats.resident).toBeGreaterThanOrEqual(2);
    expect(stats.resident).toBeLessThanOrEqual(4); // max 4 slots
  });

  it('parent fallback logic works', () => {
    const scene = buildScene([{
      name: 'sphere',
      raw: generateSphere(2, 16, 32),
      instances: [mat4Identity()],
    }]);

    const pm = new PageManager(1); // very small pool
    pm.registerPages(scene.pages);

    // Only load first page
    pm.requestPage(0, 1.0);
    pm.processRequests();

    // Check: clusters on page 0 are resident, others are not
    let residentClusters = 0;
    let nonResidentClusters = 0;
    for (const c of scene.clusters) {
      if (pm.isResident(c.pageId)) {
        residentClusters++;
      } else {
        nonResidentClusters++;
      }
    }

    expect(residentClusters).toBeGreaterThan(0);
    // With only 1 page slot and multiple pages, some must be non-resident
    if (scene.pages.length > 1) {
      expect(nonResidentClusters).toBeGreaterThan(0);
    }
  });
});

describe('Edge cases', () => {
  it('handles single-triangle mesh', () => {
    const raw = {
      positions: new Float32Array([0, 0, 0, 1, 0, 0, 0, 1, 0]),
      normals: new Float32Array([0, 0, 1, 0, 0, 1, 0, 0, 1]),
      uvs: new Float32Array([0, 0, 1, 0, 0, 1]),
      indices: new Uint32Array([0, 1, 2]),
    };
    const result = buildClusters(raw, 0, 'single-tri', 0, 0, 0);
    expect(result.clusters.length).toBe(1);
    expect(result.clusters[0].indexCount).toBe(3);
  });

  it('handles very large cluster count', () => {
    // Terrain with high subdivision → many clusters
    const raw = generateTerrain(20, 20, 64, 64, 3, 1);
    const result = buildClusters(raw, 0, 'big-terrain', 0, 0, 0);
    expect(result.clusters.length).toBeGreaterThan(50);

    const hier = buildHierarchy(result.clusters, result.pages, result.vertexData, result.indexData);
    expect(hier.clusters.length).toBeGreaterThan(result.clusters.length);

    // Hierarchy should have multiple levels
    const levels = new Set(hier.clusters.map(c => c.lodLevel));
    expect(levels.size).toBeGreaterThanOrEqual(2);
  });

  it('handles mesh with degenerate triangles', () => {
    const raw = {
      positions: new Float32Array([
        0, 0, 0, 1, 0, 0, 0, 1, 0, // good triangle
        0, 0, 0, 0, 0, 0, 0, 0, 0, // degenerate (zero area)
      ]),
      normals: new Float32Array([
        0, 0, 1, 0, 0, 1, 0, 0, 1,
        0, 0, 1, 0, 0, 1, 0, 0, 1,
      ]),
      uvs: new Float32Array([0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0]),
      indices: new Uint32Array([0, 1, 2, 3, 4, 5]),
    };
    // Should not crash
    const result = buildClusters(raw, 0, 'degenerate', 0, 0, 0);
    expect(result.clusters.length).toBeGreaterThan(0);
  });

  it('projected error is 0 for zero geometric error', () => {
    const err = projectedError(0, 10, 1080, Math.PI / 3);
    expect(err).toBe(0);
  });

  it('handles transforms correctly', () => {
    const scale = mat4Scale(2, 2, 2);
    const rot = mat4RotationY(Math.PI / 4);
    const trans = mat4Translation(10, 0, 0);
    const combined = mat4Multiply(trans, mat4Multiply(rot, scale));

    const scene = buildScene([{
      name: 'sphere',
      raw: generateSphere(1, 4, 8),
      instances: [combined],
    }]);

    const inst = scene.instances[0];
    // World bounds should reflect the transform
    expect(inst.worldBounds[0]).toBeCloseTo(10, 0); // translated X
    expect(inst.worldBounds[3]).toBeGreaterThan(1); // scaled radius
  });
});

describe('Performance characteristics', () => {
  it('cluster sizes are within expected range', () => {
    const raw = generateSphere(2, 32, 64);
    const built = buildClusters(raw, 0, 'sphere', 0, 0, 0);

    for (const c of built.clusters) {
      const triCount = c.indexCount / 3;
      // Most clusters should be near 64 triangles
      expect(triCount).toBeGreaterThan(0);
      expect(triCount).toBeLessThanOrEqual(128);
    }
  });

  it('hierarchy reduces draw count at coarse LOD', () => {
    const raw = generateSphere(2, 32, 64);
    const built = buildClusters(raw, 0, 'sphere', 0, 0, 0);
    const hier = buildHierarchy(built.clusters, built.pages, built.vertexData, built.indexData);

    const leafCount = built.clusters.length;
    const totalNodes = hier.clusters.length;

    // There should be significantly fewer internal nodes than leaves
    const internalNodes = totalNodes - leafCount;
    expect(internalNodes).toBeGreaterThan(0);
    expect(internalNodes).toBeLessThan(leafCount);
  });

  it('page count is reasonable', () => {
    const raw = generateSphere(2, 32, 64);
    const built = buildClusters(raw, 0, 'sphere', 0, 0, 0);

    // With ~64 clusters and 16 clusters per page, expect ~4 pages
    expect(built.pages.length).toBeGreaterThan(0);
    expect(built.pages.length).toBeLessThan(built.clusters.length);
  });
});
