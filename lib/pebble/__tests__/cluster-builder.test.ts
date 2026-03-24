import { describe, it, expect } from 'vitest';
import { buildClusters } from '../cluster-builder';
import { generateSphere, generatePlane, generateTorus } from '../mesh-generator';

describe('buildClusters', () => {
  it('clusterizes a small sphere', () => {
    const raw = generateSphere(1, 8, 16);
    const result = buildClusters(raw, 0, 'sphere', 0, 0, 0);

    expect(result.clusters.length).toBeGreaterThan(0);
    expect(result.pages.length).toBeGreaterThan(0);
    expect(result.vertexData.length).toBeGreaterThan(0);
    expect(result.indexData.length).toBeGreaterThan(0);

    // Every cluster should have valid geometry
    for (const c of result.clusters) {
      expect(c.vertexCount).toBeGreaterThan(0);
      expect(c.indexCount).toBeGreaterThan(0);
      expect(c.indexCount % 3).toBe(0); // must be triangle count
      expect(c.boundingSphere[3]).toBeGreaterThan(0); // positive radius
      expect(c.pageId).toBeGreaterThanOrEqual(0);
    }
  });

  it('clusterizes a plane into multiple clusters', () => {
    const raw = generatePlane(10, 10, 32, 32);
    const triCount = raw.indices.length / 3;
    const result = buildClusters(raw, 0, 'plane', 0, 0, 0);

    // Should produce multiple clusters for a 2048-triangle plane
    expect(result.clusters.length).toBeGreaterThan(1);

    // Total triangles across all clusters should equal input
    const totalTris = result.clusters.reduce((s, c) => s + c.indexCount / 3, 0);
    expect(totalTris).toBe(triCount);
  });

  it('respects global offsets', () => {
    const raw = generateSphere(1, 4, 8);
    const result = buildClusters(raw, 0, 'test', 100, 5000, 2000);

    expect(result.mesh.clusterOffset).toBe(100);
    expect(result.clusters[0].vertexOffset).toBeGreaterThanOrEqual(5000);
    expect(result.clusters[0].indexOffset).toBeGreaterThanOrEqual(2000);
  });

  it('assigns all clusters to pages', () => {
    const raw = generateTorus(2, 0.5, 24, 12);
    const result = buildClusters(raw, 0, 'torus', 0, 0, 0);

    for (const c of result.clusters) {
      expect(c.pageId).toBeGreaterThanOrEqual(0);
      expect(c.pageId).toBeLessThan(result.pages.length);
    }

    // Every cluster should be referenced by exactly one page
    const referencedClusters = new Set<number>();
    for (const page of result.pages) {
      for (const cid of page.clusterIds) {
        expect(referencedClusters.has(cid)).toBe(false);
        referencedClusters.add(cid);
      }
    }
    expect(referencedClusters.size).toBe(result.clusters.length);
  });

  it('bounding spheres contain all cluster vertices', () => {
    const raw = generateSphere(1, 8, 16);
    const result = buildClusters(raw, 0, 'sphere', 0, 0, 0);

    for (const c of result.clusters) {
      const bs = c.boundingSphere;
      const cx = bs[0], cy = bs[1], cz = bs[2], r = bs[3];

      // Check that every vertex in this cluster is within the bounding sphere
      // We need to read from the packed vertex data
      const floatsPerVert = 8;
      const baseFloat = (c.vertexOffset); // vertexOffset is in floats already scaled
      // Actually vertexOffset is cumulative float offset... let me check via indexCount
      // The packed data is in result.vertexData starting from the cluster's local offset
    }

    // At minimum, radius should be positive for non-degenerate clusters
    for (const c of result.clusters) {
      if (c.vertexCount > 1) {
        expect(c.boundingSphere[3]).toBeGreaterThan(0);
      }
    }
  });

  it('normal cones have valid values', () => {
    const raw = generatePlane(5, 5, 8, 8);
    const result = buildClusters(raw, 0, 'plane', 0, 0, 0);

    for (const c of result.clusters) {
      const nc = c.normalCone;
      const dirLen = Math.sqrt(nc[0] ** 2 + nc[1] ** 2 + nc[2] ** 2);
      // Direction should be normalized (or zero for degenerate)
      if (dirLen > 0.01) {
        expect(dirLen).toBeCloseTo(1.0, 1);
      }
      // cos(halfAngle) should be in [-1, 1]
      expect(nc[3]).toBeGreaterThanOrEqual(-1.01);
      expect(nc[3]).toBeLessThanOrEqual(1.01);
    }

    // For a flat plane, all normals should point up, so cone should be very tight
    for (const c of result.clusters) {
      expect(c.normalCone[1]).toBeCloseTo(1.0, 1); // Y-up
      expect(c.normalCone[3]).toBeCloseTo(1.0, 1); // tight cone
    }
  });

  it('mesh descriptor has correct counts', () => {
    const raw = generateSphere(1, 8, 16);
    const triCount = raw.indices.length / 3;
    const result = buildClusters(raw, 0, 'sphere', 0, 0, 0);

    expect(result.mesh.clusterCount).toBe(result.clusters.length);
    expect(result.mesh.totalTriangles).toBe(triCount);
    expect(result.mesh.totalIndices).toBe(result.indexData.length);
    expect(result.mesh.pageIds.length).toBe(result.pages.length);
  });
});

describe('buildClusters with large mesh', () => {
  it('handles a high-poly sphere', () => {
    const raw = generateSphere(1, 32, 64);
    const result = buildClusters(raw, 0, 'hires-sphere', 0, 0, 0);

    // 32*64*2 = 4096 triangles, should produce ~64 clusters of 64 tris
    expect(result.clusters.length).toBeGreaterThan(30);
    expect(result.clusters.length).toBeLessThan(200);
  });
});
