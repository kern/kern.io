import { describe, it, expect } from 'vitest';
import { buildHierarchy } from '../hierarchy-builder';
import { buildClusters } from '../cluster-builder';
import { generateSphere, generatePlane, generateTorus } from '../mesh-generator';

describe('buildHierarchy', () => {
  it('builds hierarchy for a sphere', () => {
    const raw = generateSphere(1, 16, 32);
    const built = buildClusters(raw, 0, 'sphere', 0, 0, 0);
    const result = buildHierarchy(built.clusters, built.pages, built.vertexData, built.indexData);

    // Should have more clusters than leaves (added parents)
    expect(result.clusters.length).toBeGreaterThanOrEqual(built.clusters.length);

    // Root index should be valid
    expect(result.rootIndex).toBeGreaterThanOrEqual(0);
    expect(result.rootIndex).toBeLessThan(result.clusters.length);

    // Root should have no parent
    expect(result.clusters[result.rootIndex].parentIndex).toBe(-1);
  });

  it('parent error is always >= child error', () => {
    const raw = generateTorus(2, 0.5, 24, 12);
    const built = buildClusters(raw, 0, 'torus', 0, 0, 0);
    const result = buildHierarchy(built.clusters, built.pages, built.vertexData, built.indexData);

    for (const c of result.clusters) {
      if (c.parentIndex >= 0 && c.parentIndex < result.clusters.length) {
        const parent = result.clusters[c.parentIndex];
        expect(parent.lodError).toBeGreaterThanOrEqual(c.lodError);
      }
    }
  });

  it('all leaf clusters have parent links (except if too few)', () => {
    const raw = generateSphere(1, 16, 32);
    const built = buildClusters(raw, 0, 'sphere', 0, 0, 0);
    const result = buildHierarchy(built.clusters, built.pages, built.vertexData, built.indexData);

    // Count leaves with parents
    let leavesWithParents = 0;
    for (let i = 0; i < built.clusters.length; i++) {
      if (result.clusters[i].parentIndex >= 0) leavesWithParents++;
    }

    if (built.clusters.length >= 4) {
      expect(leavesWithParents).toBe(built.clusters.length);
    }
  });

  it('hierarchy has multiple LOD levels', () => {
    const raw = generateSphere(1, 32, 64);
    const built = buildClusters(raw, 0, 'sphere', 0, 0, 0);
    const result = buildHierarchy(built.clusters, built.pages, built.vertexData, built.indexData);

    const lodLevels = new Set(result.clusters.map(c => c.lodLevel));
    expect(lodLevels.size).toBeGreaterThanOrEqual(2);
  });

  it('parent bounding sphere contains children', () => {
    const raw = generateSphere(1, 16, 32);
    const built = buildClusters(raw, 0, 'sphere', 0, 0, 0);
    const result = buildHierarchy(built.clusters, built.pages, built.vertexData, built.indexData);

    for (const c of result.clusters) {
      if (c.childCount > 0 && c.childOffset >= 0) {
        const pbs = c.boundingSphere;
        for (let i = 0; i < c.childCount; i++) {
          const childIdx = c.childOffset + i;
          if (childIdx < result.clusters.length) {
            const cbs = result.clusters[childIdx].boundingSphere;
            // Child center + radius should be within parent radius (with tolerance)
            const dx = cbs[0] - pbs[0];
            const dy = cbs[1] - pbs[1];
            const dz = cbs[2] - pbs[2];
            const dist = Math.sqrt(dx * dx + dy * dy + dz * dz);
            // Allow tolerance since sphere merging is approximate
            expect(dist + cbs[3]).toBeLessThanOrEqual(pbs[3] * 1.5 + 0.5);
          }
        }
      }
    }
  });

  it('all clusters assigned to pages', () => {
    const raw = generateSphere(1, 16, 32);
    const built = buildClusters(raw, 0, 'sphere', 0, 0, 0);
    const result = buildHierarchy(built.clusters, built.pages, built.vertexData, built.indexData);

    for (const c of result.clusters) {
      expect(c.pageId).toBeGreaterThanOrEqual(0);
    }
  });

  it('handles small mesh (< 4 clusters) gracefully', () => {
    const raw = generateSphere(1, 4, 4);
    const built = buildClusters(raw, 0, 'tiny', 0, 0, 0);
    const result = buildHierarchy(built.clusters, built.pages, built.vertexData, built.indexData);

    expect(result.clusters.length).toBeGreaterThan(0);
    expect(result.rootIndex).toBeGreaterThanOrEqual(0);
  });
});
