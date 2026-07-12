/**
 * Backface culling correctness tests.
 *
 * The backface-cull pass computes face normals via cross(e1, e2) from the
 * mesh's index winding.  If the winding produces inward-pointing normals the
 * per-cluster normal cone will point inward, causing the GPU cull test to
 * eliminate front-facing clusters and keep back-facing ones — the opposite of
 * the intended behaviour.
 *
 * This suite:
 *  1. Checks face-normal direction for every generator (face normal must agree
 *     in direction with the stored vertex normal / expected outward direction).
 *  2. Checks that the built normal cones for a sphere point outward.
 *  3. Simulates the GPU cull test in TS and verifies correct visibility.
 *  4. Produces snapshot tables so failures are easy to read.
 */

import { describe, it, expect } from 'vitest';
import {
  generateSphere,
  generatePlane,
  generateTorus,
  generateTerrain,
  generateTrefoilKnot,
  generateOcean,
  generateTree,
  generateMountains,
} from '../mesh-generator';
import { buildClusters } from '../cluster-builder';
import type { Cluster } from '../types';
import { v3normalize, v3cross, v3sub, v3dot, v3len, Vec3 } from '../math';

// ─── helpers ────────────────────────────────────────────────────────────────

/** Face normal of triangle (i0,i1,i2) in a flat positions array. */
function faceNormal(pos: Float32Array, i0: number, i1: number, i2: number): Vec3 {
  const v0: Vec3 = [pos[i0 * 3], pos[i0 * 3 + 1], pos[i0 * 3 + 2]];
  const v1: Vec3 = [pos[i1 * 3], pos[i1 * 3 + 1], pos[i1 * 3 + 2]];
  const v2: Vec3 = [pos[i2 * 3], pos[i2 * 3 + 1], pos[i2 * 3 + 2]];
  return v3normalize(v3cross(v3sub(v1, v0), v3sub(v2, v0)));
}

/** Average vertex normal across a triangle (using stored normals array). */
function avgVertexNormal(nrm: Float32Array, i0: number, i1: number, i2: number): Vec3 {
  return v3normalize([
    (nrm[i0 * 3] + nrm[i1 * 3] + nrm[i2 * 3]) / 3,
    (nrm[i0 * 3 + 1] + nrm[i1 * 3 + 1] + nrm[i2 * 3 + 1]) / 3,
    (nrm[i0 * 3 + 2] + nrm[i1 * 3 + 2] + nrm[i2 * 3 + 2]) / 3,
  ]);
}

/**
 * For every triangle in the mesh check that the face normal (from cross product)
 * agrees in direction with the stored vertex normal (dot > 0).
 * Returns { passCount, failCount, details } where details lists failing triangle
 * indices for snapshot inspection.
 */
function checkWindingOrder(
  pos: Float32Array,
  nrm: Float32Array,
  idx: Uint32Array,
): { passCount: number; failCount: number; sampleFailing: string } {
  let passCount = 0;
  let failCount = 0;
  const failSamples: string[] = [];
  const triCount = idx.length / 3;

  for (let t = 0; t < triCount; t++) {
    const i0 = idx[t * 3], i1 = idx[t * 3 + 1], i2 = idx[t * 3 + 2];
    const fn = faceNormal(pos, i0, i1, i2);
    // Skip degenerate triangles (two coincident vertices → zero cross product)
    if (v3len(fn) < 0.5) continue;
    const vn = avgVertexNormal(nrm, i0, i1, i2);
    const d = v3dot(fn, vn);
    if (d > 0) {
      passCount++;
    } else {
      failCount++;
      if (failSamples.length < 3) {
        failSamples.push(
          `tri[${t}] faceN=(${fn.map(x => x.toFixed(2))}) vertN=(${vn.map(x => x.toFixed(2))}) dot=${d.toFixed(3)}`,
        );
      }
    }
  }
  return { passCount, failCount, sampleFailing: failSamples.join('\n') };
}

/**
 * Simulate the WGSL cluster backface cull test.
 * Returns true if the cluster is culled (all faces backfacing).
 */
function simulateCull(c: Cluster, cameraPos: Vec3): boolean {
  const cx = c.boundingSphere[0], cy = c.boundingSphere[1], cz = c.boundingSphere[2];
  const coneDir: Vec3 = [c.normalCone[0], c.normalCone[1], c.normalCone[2]];
  const coneCos = c.normalCone[3];

  const dx = cx - cameraPos[0], dy = cy - cameraPos[1], dz = cz - cameraPos[2];
  const len = Math.sqrt(dx * dx + dy * dy + dz * dz);
  if (len < 1e-6) return false;
  const viewDir: Vec3 = [dx / len, dy / len, dz / len];

  const coneLen = v3len(coneDir);
  if (coneLen > 0.01 && coneCos > 0.0) {
    const d = v3dot(viewDir, coneDir);
    if (d > Math.sqrt(1.0 - coneCos * coneCos)) return true;
  }
  return false;
}

/**
 * Build a short visibility "snapshot" string for a set of clusters viewed from
 * a camera position.  '#' = visible, '.' = culled.
 */
function visibilitySnapshot(clusters: Cluster[], cameraPos: Vec3): string {
  return clusters.map(c => (simulateCull(c, cameraPos) ? '.' : '#')).join('');
}

// ─── winding order ───────────────────────────────────────────────────────────

describe('Winding order: face normals must agree with stored vertex normals', () => {
  it('generatePlane: outward (+Y) face normals', () => {
    const m = generatePlane(10, 10, 4, 4);
    const r = checkWindingOrder(m.positions, m.normals, m.indices);
    expect(r.failCount).toBe(0);
  });

  it('generateTerrain: outward face normals', () => {
    const m = generateTerrain(20, 20, 8, 8, 1, 0.1);
    const r = checkWindingOrder(m.positions, m.normals, m.indices);
    expect(r.failCount).toBe(0);
  });

  it('generateOcean: outward face normals', () => {
    const m = generateOcean(100, 16, 1.2);
    const r = checkWindingOrder(m.positions, m.normals, m.indices);
    expect(r.failCount).toBe(0);
  });

  it('generateMountains: outward face normals', () => {
    const m = generateMountains(80, 16, 20);
    const r = checkWindingOrder(m.positions, m.normals, m.indices);
    expect(r.failCount).toBe(0);
  });

  it('generateTree: outward face normals', () => {
    const m = generateTree();
    const r = checkWindingOrder(m.positions, m.normals, m.indices);
    expect(r.failCount).toBe(0);
  });

  it('generateSphere: outward face normals', () => {
    const m = generateSphere(1, 8, 16);
    const r = checkWindingOrder(m.positions, m.normals, m.indices);
    if (r.failCount > 0) {
      console.log(`FAIL: ${r.failCount}/${r.passCount + r.failCount} triangles have INWARD face normals`);
      console.log(r.sampleFailing);
    }
    expect(r.failCount).toBe(0);
  });

  it('generateTorus: outward face normals', () => {
    const m = generateTorus(2, 0.5, 8, 8);
    const r = checkWindingOrder(m.positions, m.normals, m.indices);
    if (r.failCount > 0) {
      console.log(`FAIL: ${r.failCount}/${r.passCount + r.failCount} triangles have INWARD face normals`);
      console.log(r.sampleFailing);
    }
    expect(r.failCount).toBe(0);
  });

  it('generateTrefoilKnot: outward face normals', () => {
    const m = generateTrefoilKnot(0.3, 2, 64, 16);
    const r = checkWindingOrder(m.positions, m.normals, m.indices);
    if (r.failCount > 0) {
      console.log(`FAIL: ${r.failCount}/${r.passCount + r.failCount} triangles have INWARD face normals`);
      console.log(r.sampleFailing);
    }
    expect(r.failCount).toBe(0);
  });
});

// ─── normal cone direction ────────────────────────────────────────────────────

describe('Normal cone direction for clustered sphere', () => {
  function buildSphere() {
    const m = generateSphere(1, 16, 32);
    return buildClusters(m, 0, 'sphere', 0, 0, 0).clusters;
  }

  it('topmost cluster has a cone pointing upward (+Y)', () => {
    const clusters = buildSphere();
    const top = clusters.reduce((best, c) =>
      c.boundingSphere[1] > best.boundingSphere[1] ? c : best,
    );
    const coneY = top.normalCone[1];
    console.log(`Top cluster: center.y=${top.boundingSphere[1].toFixed(3)}, coneDir.y=${coneY.toFixed(3)}, coneCos=${top.normalCone[3].toFixed(3)}`);
    expect(coneY).toBeGreaterThan(0);
  });

  it('bottommost cluster has a cone pointing downward (-Y)', () => {
    const clusters = buildSphere();
    const bot = clusters.reduce((best, c) =>
      c.boundingSphere[1] < best.boundingSphere[1] ? c : best,
    );
    const coneY = bot.normalCone[1];
    console.log(`Bottom cluster: center.y=${bot.boundingSphere[1].toFixed(3)}, coneDir.y=${coneY.toFixed(3)}, coneCos=${bot.normalCone[3].toFixed(3)}`);
    expect(coneY).toBeLessThan(0);
  });
});

// ─── backface cull simulation ─────────────────────────────────────────────────

describe('Backface cull simulation — sphere', () => {
  const SPHERE_CAMERA_DIST = 10;

  function buildLeafClusters() {
    const m = generateSphere(1, 16, 32);
    return buildClusters(m, 0, 'sphere', 0, 0, 0).clusters;
  }

  it('topmost cluster is visible from above, culled from below', () => {
    const clusters = buildLeafClusters();
    const top = clusters.reduce((best, c) =>
      c.boundingSphere[1] > best.boundingSphere[1] ? c : best,
    );

    const fromAbove: Vec3 = [0, SPHERE_CAMERA_DIST, 0];
    const fromBelow: Vec3 = [0, -SPHERE_CAMERA_DIST, 0];

    const cullAbove = simulateCull(top, fromAbove);
    const cullBelow = simulateCull(top, fromBelow);
    console.log(`Top cluster: visibleFromAbove=${!cullAbove}, visibleFromBelow=${!cullBelow}`);
    expect(cullAbove).toBe(false); // must be visible
    expect(cullBelow).toBe(true);  // must be culled
  });

  it('bottommost cluster is culled from above, visible from below', () => {
    const clusters = buildLeafClusters();
    const bot = clusters.reduce((best, c) =>
      c.boundingSphere[1] < best.boundingSphere[1] ? c : best,
    );

    const fromAbove: Vec3 = [0, SPHERE_CAMERA_DIST, 0];
    const fromBelow: Vec3 = [0, -SPHERE_CAMERA_DIST, 0];

    const cullAbove = simulateCull(bot, fromAbove);
    const cullBelow = simulateCull(bot, fromBelow);
    console.log(`Bottom cluster: visibleFromAbove=${!cullAbove}, visibleFromBelow=${!cullBelow}`);
    expect(cullAbove).toBe(true);  // must be culled
    expect(cullBelow).toBe(false); // must be visible
  });

  it('roughly half the clusters culled from any axis-aligned camera', () => {
    // Use a finer sphere so clusters are small enough to lie in a single
    // hemisphere.  Coarse spheres produce large equatorial clusters that span
    // both hemispheres and are never culled, artificially lowering the ratio.
    const fineClusters = buildClusters(
      generateSphere(1, 32, 64), 0, 'fine-sphere', 0, 0, 0,
    ).clusters;
    const cameras: Array<[string, Vec3]> = [
      ['+Y', [0, SPHERE_CAMERA_DIST, 0]],
      ['-Y', [0, -SPHERE_CAMERA_DIST, 0]],
      ['+X', [SPHERE_CAMERA_DIST, 0, 0]],
      ['-X', [-SPHERE_CAMERA_DIST, 0, 0]],
      ['+Z', [0, 0, SPHERE_CAMERA_DIST]],
      ['-Z', [0, 0, -SPHERE_CAMERA_DIST]],
    ];

    for (const [label, cam] of cameras) {
      const culled = fineClusters.filter(c => simulateCull(c, cam)).length;
      const ratio = culled / fineClusters.length;
      console.log(`Camera ${label}: ${culled}/${fineClusters.length} culled (${(ratio * 100).toFixed(0)}%)`);
      // Between 25% and 75% should be culled (the back hemisphere)
      expect(ratio).toBeGreaterThan(0.25);
      expect(ratio).toBeLessThan(0.75);
    }
  });

  it('no cluster is culled from ALL 6 axis directions', () => {
    const clusters = buildLeafClusters();
    const cameras: Vec3[] = [
      [0, SPHERE_CAMERA_DIST, 0],
      [0, -SPHERE_CAMERA_DIST, 0],
      [SPHERE_CAMERA_DIST, 0, 0],
      [-SPHERE_CAMERA_DIST, 0, 0],
      [0, 0, SPHERE_CAMERA_DIST],
      [0, 0, -SPHERE_CAMERA_DIST],
    ];

    const alwaysCulled = clusters.filter(c => cameras.every(cam => simulateCull(c, cam)));
    expect(alwaysCulled.length).toBe(0);
  });

  it('no cluster is visible from ALL 6 axis directions (some hemisphere must be culled)', () => {
    const clusters = buildLeafClusters();
    const cameras: Vec3[] = [
      [0, SPHERE_CAMERA_DIST, 0],
      [0, -SPHERE_CAMERA_DIST, 0],
      [SPHERE_CAMERA_DIST, 0, 0],
      [-SPHERE_CAMERA_DIST, 0, 0],
      [0, 0, SPHERE_CAMERA_DIST],
      [0, 0, -SPHERE_CAMERA_DIST],
    ];

    const alwaysVisible = clusters.filter(c => cameras.every(cam => !simulateCull(c, cam)));
    // At most 1 cluster (e.g. edge cluster between hemispheres) might be visible from all 6
    expect(alwaysVisible.length).toBeLessThanOrEqual(2);
  });
});

// ─── snapshot grid ────────────────────────────────────────────────────────────

describe('Visibility snapshot grid — sphere from 6 camera directions', () => {
  it('snapshot: camera ±Y produce complementary visibility', () => {
    // Fine sphere so clusters are small and confined to one hemisphere.
    const m = generateSphere(1, 32, 64);
    const { clusters } = buildClusters(m, 0, 'sphere', 0, 0, 0);

    const above: Vec3 = [0, 10, 0];
    const below: Vec3 = [0, -10, 0];

    const snapAbove = visibilitySnapshot(clusters, above);
    const snapBelow = visibilitySnapshot(clusters, below);

    console.log('\n=== Visibility snapshot (# = visible, . = culled) ===');
    console.log(`From +Y (above): ${snapAbove}`);
    console.log(`From -Y (below): ${snapBelow}`);

    // Cluster visible from above should not be visible from below and vice-versa
    // (with some tolerance for edge clusters)
    let bothVisible = 0;
    let bothCulled = 0;
    for (let i = 0; i < clusters.length; i++) {
      if (snapAbove[i] === '#' && snapBelow[i] === '#') bothVisible++;
      if (snapAbove[i] === '.' && snapBelow[i] === '.') bothCulled++;
    }

    const n = clusters.length;
    console.log(`Both visible: ${bothVisible}/${n}, both culled: ${bothCulled}/${n}`);

    // At most ~25% of clusters can be visible from both sides (equatorial
    // clusters straddle both hemispheres even with a finer sphere).
    expect(bothVisible / n).toBeLessThan(0.25);
    // No cluster should be culled from both sides
    expect(bothCulled).toBe(0);
  });
});

// ─── backface cull simulation — torus ─────────────────────────────────────────

describe('Backface cull simulation — torus', () => {
  it('outer-rim clusters are never incorrectly culled when viewed from outside', () => {
    // Camera far outside on +X — outer-rim clusters face toward camera and
    // must NOT be culled (correctness: never hide a visible surface).
    const m = generateTorus(2, 0.5, 24, 12);
    const { clusters } = buildClusters(m, 0, 'torus', 0, 0, 0);

    const outside: Vec3 = [20, 0, 0];
    const outerClusters = clusters.filter(c => c.boundingSphere[0] > 1.5);
    if (outerClusters.length === 0) return;

    let wrongFromOutside = 0;
    for (const c of outerClusters) {
      if (simulateCull(c, outside)) wrongFromOutside++;
    }

    console.log(`Outer torus clusters: ${outerClusters.length}, wrongly culled from outside: ${wrongFromOutside}`);
    expect(wrongFromOutside).toBe(0);
  });

  it('fine-resolution torus: outer-rim clusters are culled when viewed from origin', () => {
    // Use a finer torus so each cluster spans only a small arc of the tube
    // minor circle — giving a tight enough cone for culling.
    // With minorSegments=64: 24×64×2=3072 triangles → ~48 clusters
    // Each cluster covers ~64/128 ≈ 0.5 major rings, so the minor coverage is
    // a fraction of the tube circumference → coneCos should be well > 0.
    const m = generateTorus(2, 0.5, 24, 64);
    const { clusters } = buildClusters(m, 0, 'fine-torus', 0, 0, 0);

    const inside: Vec3 = [0, 0, 0]; // inside the torus hole
    // Use a strict filter: center.x > majorRadius (2.0) means the cluster
    // sits clearly on the outer half of the torus where all tube faces point
    // away from the origin.
    const outerClusters = clusters.filter(c => c.boundingSphere[0] > 2.0);
    if (outerClusters.length === 0) return;

    let wrongFromInside = 0;
    for (const c of outerClusters) {
      if (!simulateCull(c, inside)) wrongFromInside++;
    }

    console.log(`Fine torus outer clusters: ${outerClusters.length}, NOT culled from origin: ${wrongFromInside}`);
    // Clusters clearly on the outer half should mostly be culled from origin
    expect(wrongFromInside / outerClusters.length).toBeLessThan(0.4);
  });
});
