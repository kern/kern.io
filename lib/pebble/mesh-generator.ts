/**
 * Procedural mesh generators for testing.
 * Generates triangle soup as RawMesh for the cluster builder.
 */

import type { RawMesh } from './cluster-builder';
import { v3normalize, v3cross, Vec3 } from './math';

/** Generate a UV sphere. */
export function generateSphere(
  radius: number = 1,
  latSegments: number = 32,
  lonSegments: number = 64,
): RawMesh {
  const positions: number[] = [];
  const normals: number[] = [];
  const uvs: number[] = [];
  const indices: number[] = [];

  for (let lat = 0; lat <= latSegments; lat++) {
    const theta = (lat * Math.PI) / latSegments;
    const sinTheta = Math.sin(theta);
    const cosTheta = Math.cos(theta);

    for (let lon = 0; lon <= lonSegments; lon++) {
      const phi = (lon * 2 * Math.PI) / lonSegments;
      const sinPhi = Math.sin(phi);
      const cosPhi = Math.cos(phi);

      const x = cosPhi * sinTheta;
      const y = cosTheta;
      const z = sinPhi * sinTheta;

      positions.push(radius * x, radius * y, radius * z);
      normals.push(x, y, z);
      uvs.push(lon / lonSegments, lat / latSegments);
    }
  }

  for (let lat = 0; lat < latSegments; lat++) {
    for (let lon = 0; lon < lonSegments; lon++) {
      const a = lat * (lonSegments + 1) + lon;
      const b = a + lonSegments + 1;
      indices.push(a, b, a + 1);
      indices.push(b, b + 1, a + 1);
    }
  }

  return {
    positions: new Float32Array(positions),
    normals: new Float32Array(normals),
    uvs: new Float32Array(uvs),
    indices: new Uint32Array(indices),
  };
}

/** Generate a subdivided plane. */
export function generatePlane(
  width: number = 10,
  depth: number = 10,
  segW: number = 64,
  segD: number = 64,
): RawMesh {
  const positions: number[] = [];
  const normals: number[] = [];
  const uvs: number[] = [];
  const indices: number[] = [];

  for (let z = 0; z <= segD; z++) {
    for (let x = 0; x <= segW; x++) {
      const u = x / segW;
      const v = z / segD;
      positions.push((u - 0.5) * width, 0, (v - 0.5) * depth);
      normals.push(0, 1, 0);
      uvs.push(u, v);
    }
  }

  for (let z = 0; z < segD; z++) {
    for (let x = 0; x < segW; x++) {
      const a = z * (segW + 1) + x;
      const b = a + segW + 1;
      indices.push(a, b, a + 1);
      indices.push(b, b + 1, a + 1);
    }
  }

  return {
    positions: new Float32Array(positions),
    normals: new Float32Array(normals),
    uvs: new Float32Array(uvs),
    indices: new Uint32Array(indices),
  };
}

/** Generate a terrain with sine-based heightmap. */
export function generateTerrain(
  width: number = 20,
  depth: number = 20,
  segW: number = 128,
  segD: number = 128,
  amplitude: number = 2,
  frequency: number = 0.5,
): RawMesh {
  const positions: number[] = [];
  const normals: number[] = [];
  const uvs: number[] = [];
  const indices: number[] = [];

  // Generate height at grid points
  const heights: number[] = [];
  for (let z = 0; z <= segD; z++) {
    for (let x = 0; x <= segW; x++) {
      const u = x / segW;
      const v = z / segD;
      const wx = (u - 0.5) * width;
      const wz = (v - 0.5) * depth;
      const h = amplitude * (
        Math.sin(wx * frequency) * Math.cos(wz * frequency) +
        0.5 * Math.sin(wx * frequency * 2.3 + 1.7) * Math.cos(wz * frequency * 1.8 + 0.3) +
        0.25 * Math.sin(wx * frequency * 4.1 + 3.2) * Math.cos(wz * frequency * 3.7 + 2.1)
      );
      heights.push(h);
      positions.push(wx, h, wz);
      uvs.push(u, v);
    }
  }

  // Compute normals via finite differences
  for (let z = 0; z <= segD; z++) {
    for (let x = 0; x <= segW; x++) {
      const idx = z * (segW + 1) + x;
      const hL = x > 0 ? heights[idx - 1] : heights[idx];
      const hR = x < segW ? heights[idx + 1] : heights[idx];
      const hD = z > 0 ? heights[idx - (segW + 1)] : heights[idx];
      const hU = z < segD ? heights[idx + (segW + 1)] : heights[idx];
      const dx = width / segW;
      const dz = depth / segD;
      const n = v3normalize([-(hR - hL) / (2 * dx), 1, -(hU - hD) / (2 * dz)]);
      normals.push(n[0], n[1], n[2]);
    }
  }

  for (let z = 0; z < segD; z++) {
    for (let x = 0; x < segW; x++) {
      const a = z * (segW + 1) + x;
      const b = a + segW + 1;
      indices.push(a, b, a + 1);
      indices.push(b, b + 1, a + 1);
    }
  }

  return {
    positions: new Float32Array(positions),
    normals: new Float32Array(normals),
    uvs: new Float32Array(uvs),
    indices: new Uint32Array(indices),
  };
}

/** Generate a torus. */
export function generateTorus(
  majorRadius: number = 2,
  minorRadius: number = 0.5,
  majorSegments: number = 48,
  minorSegments: number = 24,
): RawMesh {
  const positions: number[] = [];
  const normals: number[] = [];
  const uvs: number[] = [];
  const indices: number[] = [];

  for (let i = 0; i <= majorSegments; i++) {
    const u = (i / majorSegments) * Math.PI * 2;
    const cu = Math.cos(u), su = Math.sin(u);

    for (let j = 0; j <= minorSegments; j++) {
      const v = (j / minorSegments) * Math.PI * 2;
      const cv = Math.cos(v), sv = Math.sin(v);

      const x = (majorRadius + minorRadius * cv) * cu;
      const y = minorRadius * sv;
      const z = (majorRadius + minorRadius * cv) * su;

      const nx = cv * cu;
      const ny = sv;
      const nz = cv * su;

      positions.push(x, y, z);
      normals.push(nx, ny, nz);
      uvs.push(i / majorSegments, j / minorSegments);
    }
  }

  for (let i = 0; i < majorSegments; i++) {
    for (let j = 0; j < minorSegments; j++) {
      const a = i * (minorSegments + 1) + j;
      const b = a + minorSegments + 1;
      indices.push(a, b, a + 1);
      indices.push(b, b + 1, a + 1);
    }
  }

  return {
    positions: new Float32Array(positions),
    normals: new Float32Array(normals),
    uvs: new Float32Array(uvs),
    indices: new Uint32Array(indices),
  };
}

/** Generate a trefoil knot — high curvature, wide normal variation, good LOD stress test. */
export function generateTrefoilKnot(
  tubeRadius: number = 0.3,
  knotScale: number = 2,
  knotSegments: number = 256,
  tubeSegments: number = 32,
): RawMesh {
  const positions: number[] = [];
  const normals: number[] = [];
  const uvs: number[] = [];
  const indices: number[] = [];

  // Trefoil knot parametric curve
  function knotPoint(t: number): Vec3 {
    const s = t * Math.PI * 2;
    return [
      knotScale * (Math.sin(s) + 2 * Math.sin(2 * s)),
      knotScale * (Math.cos(s) - 2 * Math.cos(2 * s)),
      knotScale * (-Math.sin(3 * s)),
    ];
  }

  for (let i = 0; i <= knotSegments; i++) {
    const t = i / knotSegments;
    const center = knotPoint(t);

    // Frenet frame
    const eps = 1e-4;
    const next = knotPoint(t + eps);
    const tangent = v3normalize([next[0] - center[0], next[1] - center[1], next[2] - center[2]]) as Vec3;

    // Arbitrary up to bootstrap normal
    const up: Vec3 = Math.abs(tangent[1]) < 0.9 ? [0, 1, 0] : [1, 0, 0];
    const binormal = v3normalize(v3cross(tangent, up)) as Vec3;
    const normal = v3normalize(v3cross(binormal, tangent)) as Vec3;

    for (let j = 0; j <= tubeSegments; j++) {
      const v = (j / tubeSegments) * Math.PI * 2;
      const cv = Math.cos(v), sv = Math.sin(v);

      const n: Vec3 = [
        cv * normal[0] + sv * binormal[0],
        cv * normal[1] + sv * binormal[1],
        cv * normal[2] + sv * binormal[2],
      ];

      positions.push(
        center[0] + tubeRadius * n[0],
        center[1] + tubeRadius * n[1],
        center[2] + tubeRadius * n[2],
      );
      normals.push(n[0], n[1], n[2]);
      uvs.push(t, j / tubeSegments);
    }
  }

  for (let i = 0; i < knotSegments; i++) {
    for (let j = 0; j < tubeSegments; j++) {
      const a = i * (tubeSegments + 1) + j;
      const b = a + tubeSegments + 1;
      indices.push(a, b, a + 1);
      indices.push(b, b + 1, a + 1);
    }
  }

  return {
    positions: new Float32Array(positions),
    normals: new Float32Array(normals),
    uvs: new Float32Array(uvs),
    indices: new Uint32Array(indices),
  };
}

/**
 * Generate an ocean surface with Gerstner wave displacement.
 * Multiple wave trains produce realistic rolling swells.
 */
export function generateOcean(
  size: number = 100,
  segments: number = 256,
  waveAmplitude: number = 1.2,
): RawMesh {
  const positions: number[] = [];
  const normals: number[] = [];
  const uvs: number[] = [];
  const indices: number[] = [];

  // Wave parameters: [amplitude, frequency, direction_x, direction_z, phase]
  const waves = [
    [waveAmplitude * 0.6,  0.08,  1.0,   0.3,  0.0 ],
    [waveAmplitude * 0.4,  0.15,  0.6,   1.0,  1.2 ],
    [waveAmplitude * 0.25, 0.25, -0.4,   1.0,  2.5 ],
    [waveAmplitude * 0.15, 0.40,  1.0,  -0.7,  0.8 ],
    [waveAmplitude * 0.10, 0.60,  0.7,   0.7,  3.1 ],
  ];

  function waveHeight(wx: number, wz: number): number {
    let h = 0;
    for (const [a, f, dx, dz] of waves) {
      const dir = Math.sqrt(dx * dx + dz * dz);
      const nx = dx / dir, nz = dz / dir;
      h += a * Math.sin((nx * wx + nz * wz) * f);
    }
    return h;
  }

  for (let zi = 0; zi <= segments; zi++) {
    for (let xi = 0; xi <= segments; xi++) {
      const u = xi / segments;
      const v = zi / segments;
      const wx = (u - 0.5) * size;
      const wz = (v - 0.5) * size;
      const wy = waveHeight(wx, wz);

      positions.push(wx, wy, wz);
      uvs.push(u * 8, v * 8); // tile UVs for detail

      // Normal via finite differences
      const eps = size / segments;
      const dhx = waveHeight(wx + eps, wz) - waveHeight(wx - eps, wz);
      const dhz = waveHeight(wx, wz + eps) - waveHeight(wx, wz - eps);
      const n = v3normalize([-dhx / (2 * eps), 1, -dhz / (2 * eps)]);
      normals.push(n[0], n[1], n[2]);
    }
  }

  for (let zi = 0; zi < segments; zi++) {
    for (let xi = 0; xi < segments; xi++) {
      const a = zi * (segments + 1) + xi;
      const b = a + segments + 1;
      indices.push(a, b, a + 1);
      indices.push(b, b + 1, a + 1);
    }
  }

  return {
    positions: new Float32Array(positions),
    normals: new Float32Array(normals),
    uvs: new Float32Array(uvs),
    indices: new Uint32Array(indices),
  };
}

/**
 * Generate a conifer tree (cone body + cylinder trunk).
 * Returns a single merged mesh suitable for instancing.
 */
export function generateTree(
  trunkRadius: number = 0.15,
  trunkHeight: number = 1.2,
  coneRadius: number = 0.9,
  coneHeight: number = 3.5,
  segments: number = 12,
  coneLayers: number = 3,
): RawMesh {
  const positions: number[] = [];
  const normals: number[] = [];
  const uvs: number[] = [];
  const indices: number[] = [];

  function addVertex(x: number, y: number, z: number, nx: number, ny: number, nz: number, u: number, v: number) {
    positions.push(x, y, z);
    normals.push(nx, ny, nz);
    uvs.push(u, v);
    return positions.length / 3 - 1;
  }

  // Trunk (cylinder)
  for (let i = 0; i <= segments; i++) {
    const angle = (i / segments) * Math.PI * 2;
    const cx = Math.cos(angle), cz = Math.sin(angle);
    addVertex(cx * trunkRadius, 0, cz * trunkRadius, cx, 0, cz, i / segments, 0);
    addVertex(cx * trunkRadius, trunkHeight, cz * trunkRadius, cx, 0, cz, i / segments, 1);
  }
  const trunkVerts = (segments + 1) * 2;
  for (let i = 0; i < segments; i++) {
    const b = i * 2;
    indices.push(b, b + 1, b + 2);
    indices.push(b + 1, b + 3, b + 2);
  }

  // Foliage: stacked cones with increasing radius going up
  for (let layer = 0; layer < coneLayers; layer++) {
    const t = layer / coneLayers;
    const baseY = trunkHeight + t * coneHeight * 0.7;
    const topY = baseY + coneHeight * (0.5 - t * 0.15);
    const r = coneRadius * (1.0 - t * 0.4);
    const baseVert = positions.length / 3;

    // Cone slope normal
    const slopeLen = Math.sqrt(coneHeight * coneHeight + coneRadius * coneRadius);
    const ny = coneRadius / slopeLen;
    const nr = coneHeight / slopeLen;

    for (let i = 0; i <= segments; i++) {
      const angle = (i / segments) * Math.PI * 2;
      const cx = Math.cos(angle), cz = Math.sin(angle);
      addVertex(cx * r, baseY, cz * r, cx * nr, ny, cz * nr, i / segments, 1);
    }
    const apexIdx = addVertex(0, topY, 0, 0, 1, 0, 0.5, 0);

    for (let i = 0; i < segments; i++) {
      indices.push(baseVert + i, apexIdx, baseVert + i + 1);
    }
  }

  return {
    positions: new Float32Array(positions),
    normals: new Float32Array(normals),
    uvs: new Float32Array(uvs),
    indices: new Uint32Array(indices),
  };
}

/**
 * Generate a rocky mountain terrain using fractal-like layered noise.
 * Much rougher than generateTerrain — good for hero background geometry.
 */
export function generateMountains(
  size: number = 80,
  segments: number = 192,
  peakHeight: number = 20,
): RawMesh {
  const positions: number[] = [];
  const normals: number[] = [];
  const uvs: number[] = [];
  const indices: number[] = [];

  function heightAt(x: number, z: number): number {
    // Layered sinusoids approximate FBM
    let h = 0;
    let amp = 1.0, freq = 0.06;
    for (let oct = 0; oct < 6; oct++) {
      h += amp * Math.sin(x * freq + oct * 1.7) * Math.cos(z * freq + oct * 2.3);
      h += amp * 0.5 * Math.sin(x * freq * 1.4 - z * freq * 0.9 + oct);
      amp *= 0.5;
      freq *= 2.1;
    }
    // Shape into mountain by multiplying with a radial envelope
    const dist = Math.sqrt(x * x + z * z) / (size * 0.5);
    const envelope = Math.max(0, 1 - dist * dist);
    return h * peakHeight * envelope;
  }

  const step = size / segments;
  for (let zi = 0; zi <= segments; zi++) {
    for (let xi = 0; xi <= segments; xi++) {
      const wx = (xi / segments - 0.5) * size;
      const wz = (zi / segments - 0.5) * size;
      const wy = heightAt(wx, wz);
      positions.push(wx, wy, wz);
      uvs.push(xi / segments, zi / segments);

      const dhx = heightAt(wx + step, wz) - heightAt(wx - step, wz);
      const dhz = heightAt(wx, wz + step) - heightAt(wx, wz - step);
      const n = v3normalize([-dhx / (2 * step), 1, -dhz / (2 * step)]);
      normals.push(n[0], n[1], n[2]);
    }
  }

  for (let zi = 0; zi < segments; zi++) {
    for (let xi = 0; xi < segments; xi++) {
      const a = zi * (segments + 1) + xi;
      const b = a + segments + 1;
      indices.push(a, b, a + 1);
      indices.push(b, b + 1, a + 1);
    }
  }

  return {
    positions: new Float32Array(positions),
    normals: new Float32Array(normals),
    uvs: new Float32Array(uvs),
    indices: new Uint32Array(indices),
  };
}

/** Generate a dense Stanford-bunny-esque test mesh: many small spheres in a grid. */
export function generateMassiveScene(
  gridSize: number = 5,
  sphereDetail: number = 16,
): RawMesh {
  const allPos: number[] = [];
  const allNorm: number[] = [];
  const allUv: number[] = [];
  const allIdx: number[] = [];
  let vertexOffset = 0;

  for (let gx = 0; gx < gridSize; gx++) {
    for (let gz = 0; gz < gridSize; gz++) {
      const sphere = generateSphere(0.4, sphereDetail, sphereDetail * 2);
      const ox = (gx - gridSize / 2) * 1.2;
      const oz = (gz - gridSize / 2) * 1.2;

      for (let i = 0; i < sphere.positions.length; i += 3) {
        allPos.push(sphere.positions[i] + ox, sphere.positions[i + 1], sphere.positions[i + 2] + oz);
        allNorm.push(sphere.normals[i], sphere.normals[i + 1], sphere.normals[i + 2]);
      }
      for (let i = 0; i < sphere.uvs.length; i++) {
        allUv.push(sphere.uvs[i]);
      }
      for (let i = 0; i < sphere.indices.length; i++) {
        allIdx.push(sphere.indices[i] + vertexOffset);
      }
      vertexOffset += sphere.positions.length / 3;
    }
  }

  return {
    positions: new Float32Array(allPos),
    normals: new Float32Array(allNorm),
    uvs: new Float32Array(allUv),
    indices: new Uint32Array(allIdx),
  };
}
