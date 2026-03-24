/**
 * Procedural mesh generators for testing.
 * Generates triangle soup as RawMesh for the cluster builder.
 */

import type { RawMesh } from './cluster-builder';
import { v3normalize, Vec3 } from './math';

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
