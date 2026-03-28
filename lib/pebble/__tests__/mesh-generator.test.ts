import { describe, it, expect } from 'vitest';
import { generateSphere, generatePlane, generateTerrain, generateTorus, generateMassiveScene } from '../mesh-generator';

describe('generateSphere', () => {
  it('produces valid mesh data', () => {
    const mesh = generateSphere(1, 8, 16);
    expect(mesh.positions.length).toBeGreaterThan(0);
    expect(mesh.positions.length % 3).toBe(0);
    expect(mesh.normals.length).toBe(mesh.positions.length);
    expect(mesh.uvs.length).toBe((mesh.positions.length / 3) * 2);
    expect(mesh.indices.length % 3).toBe(0);
  });

  it('all vertices are on the sphere surface', () => {
    const r = 2;
    const mesh = generateSphere(r, 16, 32);
    for (let i = 0; i < mesh.positions.length; i += 3) {
      const d = Math.sqrt(
        mesh.positions[i] ** 2 + mesh.positions[i + 1] ** 2 + mesh.positions[i + 2] ** 2,
      );
      expect(d).toBeCloseTo(r, 4);
    }
  });

  it('normals are unit length', () => {
    const mesh = generateSphere(1, 8, 16);
    for (let i = 0; i < mesh.normals.length; i += 3) {
      const len = Math.sqrt(
        mesh.normals[i] ** 2 + mesh.normals[i + 1] ** 2 + mesh.normals[i + 2] ** 2,
      );
      expect(len).toBeCloseTo(1.0, 4);
    }
  });

  it('indices reference valid vertices', () => {
    const mesh = generateSphere(1, 8, 16);
    const vertCount = mesh.positions.length / 3;
    for (let i = 0; i < mesh.indices.length; i++) {
      expect(mesh.indices[i]).toBeGreaterThanOrEqual(0);
      expect(mesh.indices[i]).toBeLessThan(vertCount);
    }
  });

  it('triangle count matches expectation', () => {
    const lat = 8, lon = 16;
    const mesh = generateSphere(1, lat, lon);
    expect(mesh.indices.length / 3).toBe(lat * lon * 2);
  });
});

describe('generatePlane', () => {
  it('all Y coordinates are 0', () => {
    const mesh = generatePlane(10, 10, 8, 8);
    for (let i = 1; i < mesh.positions.length; i += 3) {
      expect(mesh.positions[i]).toBe(0);
    }
  });

  it('all normals point up', () => {
    const mesh = generatePlane(5, 5, 4, 4);
    for (let i = 0; i < mesh.normals.length; i += 3) {
      expect(mesh.normals[i]).toBe(0);
      expect(mesh.normals[i + 1]).toBe(1);
      expect(mesh.normals[i + 2]).toBe(0);
    }
  });
});

describe('generateTerrain', () => {
  it('produces valid mesh with height variation', () => {
    const mesh = generateTerrain(10, 10, 16, 16, 2, 1);
    let minY = Infinity, maxY = -Infinity;
    for (let i = 1; i < mesh.positions.length; i += 3) {
      minY = Math.min(minY, mesh.positions[i]);
      maxY = Math.max(maxY, mesh.positions[i]);
    }
    expect(maxY - minY).toBeGreaterThan(0.5);
  });
});

describe('generateTorus', () => {
  it('produces correct triangle count', () => {
    const maj = 12, min = 8;
    const mesh = generateTorus(2, 0.5, maj, min);
    expect(mesh.indices.length / 3).toBe(maj * min * 2);
  });
});

describe('generateMassiveScene', () => {
  it('produces many triangles', () => {
    const mesh = generateMassiveScene(3, 8);
    expect(mesh.indices.length / 3).toBeGreaterThan(1000);
  });
});
