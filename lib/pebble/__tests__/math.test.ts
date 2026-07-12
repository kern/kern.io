import { describe, it, expect } from 'vitest';
import {
  v3add, v3sub, v3scale, v3dot, v3cross, v3len, v3normalize, v3dist,
  mat4Identity, mat4Perspective, mat4LookAt, mat4Multiply, mat4Inverse,
  mat4TransformPoint, mat4Translation, mat4RotationY,
  extractFrustumPlanes, sphereOutsideFrustum, projectedError,
} from '../math';

const approx = (a: number, b: number, eps = 1e-5) => Math.abs(a - b) < eps;

describe('Vec3 operations', () => {
  it('v3add', () => {
    expect(v3add([1, 2, 3], [4, 5, 6])).toEqual([5, 7, 9]);
  });

  it('v3sub', () => {
    expect(v3sub([5, 7, 9], [4, 5, 6])).toEqual([1, 2, 3]);
  });

  it('v3scale', () => {
    expect(v3scale([1, 2, 3], 2)).toEqual([2, 4, 6]);
  });

  it('v3dot', () => {
    expect(v3dot([1, 0, 0], [0, 1, 0])).toBe(0);
    expect(v3dot([1, 2, 3], [4, 5, 6])).toBe(32);
  });

  it('v3cross', () => {
    const c = v3cross([1, 0, 0], [0, 1, 0]);
    expect(c[0]).toBeCloseTo(0);
    expect(c[1]).toBeCloseTo(0);
    expect(c[2]).toBeCloseTo(1);
  });

  it('v3len', () => {
    expect(v3len([3, 4, 0])).toBeCloseTo(5);
  });

  it('v3normalize', () => {
    const n = v3normalize([3, 0, 0]);
    expect(n[0]).toBeCloseTo(1);
    expect(n[1]).toBeCloseTo(0);
    expect(n[2]).toBeCloseTo(0);
  });

  it('v3normalize zero vector', () => {
    expect(v3normalize([0, 0, 0])).toEqual([0, 0, 0]);
  });

  it('v3dist', () => {
    expect(v3dist([0, 0, 0], [3, 4, 0])).toBeCloseTo(5);
  });
});

describe('Mat4 operations', () => {
  it('identity * identity = identity', () => {
    const id = mat4Identity();
    const result = mat4Multiply(id, id);
    for (let i = 0; i < 16; i++) {
      expect(result[i]).toBeCloseTo(id[i]);
    }
  });

  it('identity transforms point unchanged', () => {
    const p = mat4TransformPoint(mat4Identity(), [1, 2, 3]);
    expect(p[0]).toBeCloseTo(1);
    expect(p[1]).toBeCloseTo(2);
    expect(p[2]).toBeCloseTo(3);
  });

  it('translation moves point', () => {
    const t = mat4Translation(10, 20, 30);
    const p = mat4TransformPoint(t, [1, 2, 3]);
    expect(p[0]).toBeCloseTo(11);
    expect(p[1]).toBeCloseTo(22);
    expect(p[2]).toBeCloseTo(33);
  });

  it('inverse of identity is identity', () => {
    const inv = mat4Inverse(mat4Identity());
    for (let i = 0; i < 16; i++) {
      expect(inv[i]).toBeCloseTo(mat4Identity()[i]);
    }
  });

  it('M * M^-1 = I', () => {
    const m = mat4Translation(3, 7, -2);
    const inv = mat4Inverse(m);
    const result = mat4Multiply(m, inv);
    const id = mat4Identity();
    for (let i = 0; i < 16; i++) {
      expect(result[i]).toBeCloseTo(id[i], 4);
    }
  });

  it('perspective produces valid matrix', () => {
    const p = mat4Perspective(Math.PI / 4, 16 / 9, 0.1, 100);
    expect(p[0]).toBeGreaterThan(0); // f/aspect
    expect(p[5]).toBeGreaterThan(0); // f
    expect(p[11]).toBe(-1);          // perspective divide
  });

  it('lookAt at origin looking down -Z', () => {
    const v = mat4LookAt([0, 0, 5], [0, 0, 0], [0, 1, 0]);
    const p = mat4TransformPoint(v, [0, 0, 0]);
    // Origin should be at (0, 0, -5) in view space
    expect(p[0]).toBeCloseTo(0);
    expect(p[1]).toBeCloseTo(0);
    expect(p[2]).toBeCloseTo(-5);
  });

  it('rotation Y by π/2', () => {
    const r = mat4RotationY(Math.PI / 2);
    const p = mat4TransformPoint(r, [1, 0, 0]);
    expect(p[0]).toBeCloseTo(0);
    expect(p[2]).toBeCloseTo(-1);
  });
});

describe('Frustum culling', () => {
  it('sphere at origin is inside standard frustum', () => {
    const vp = mat4Multiply(
      mat4Perspective(Math.PI / 3, 1, 0.1, 100),
      mat4LookAt([0, 0, 5], [0, 0, 0], [0, 1, 0]),
    );
    const planes = extractFrustumPlanes(vp);
    expect(sphereOutsideFrustum(planes, 0, 0, 0, 1)).toBe(false);
  });

  it('sphere far behind camera is outside frustum', () => {
    const vp = mat4Multiply(
      mat4Perspective(Math.PI / 3, 1, 0.1, 100),
      mat4LookAt([0, 0, 5], [0, 0, 0], [0, 1, 0]),
    );
    const planes = extractFrustumPlanes(vp);
    expect(sphereOutsideFrustum(planes, 0, 0, 200, 1)).toBe(true);
  });

  it('sphere far to the right is outside frustum', () => {
    const vp = mat4Multiply(
      mat4Perspective(Math.PI / 3, 1, 0.1, 100),
      mat4LookAt([0, 0, 5], [0, 0, 0], [0, 1, 0]),
    );
    const planes = extractFrustumPlanes(vp);
    expect(sphereOutsideFrustum(planes, 1000, 0, 0, 1)).toBe(true);
  });
});

describe('projectedError', () => {
  it('returns large value for close objects', () => {
    const e = projectedError(1.0, 0.001, 1080, Math.PI / 3);
    expect(e).toBeGreaterThan(1000);
  });

  it('returns smaller value for far objects', () => {
    const near = projectedError(1.0, 1, 1080, Math.PI / 3);
    const far = projectedError(1.0, 100, 1080, Math.PI / 3);
    expect(near).toBeGreaterThan(far);
  });

  it('scales linearly with geometric error', () => {
    const e1 = projectedError(1.0, 10, 1080, Math.PI / 3);
    const e2 = projectedError(2.0, 10, 1080, Math.PI / 3);
    expect(e2 / e1).toBeCloseTo(2.0);
  });
});
