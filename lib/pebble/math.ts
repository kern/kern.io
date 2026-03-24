/**
 * Minimal linear-algebra helpers. No dependencies.
 */

export type Vec3 = [number, number, number];
export type Vec4 = [number, number, number, number];
export type Mat4 = Float32Array; // 16 elements, column-major

// ─── Vec3 ───────────────────────────────────────────────────────────────────

export function v3add(a: Vec3, b: Vec3): Vec3 {
  return [a[0] + b[0], a[1] + b[1], a[2] + b[2]];
}

export function v3sub(a: Vec3, b: Vec3): Vec3 {
  return [a[0] - b[0], a[1] - b[1], a[2] - b[2]];
}

export function v3scale(a: Vec3, s: number): Vec3 {
  return [a[0] * s, a[1] * s, a[2] * s];
}

export function v3dot(a: Vec3, b: Vec3): number {
  return a[0] * b[0] + a[1] * b[1] + a[2] * b[2];
}

export function v3cross(a: Vec3, b: Vec3): Vec3 {
  return [
    a[1] * b[2] - a[2] * b[1],
    a[2] * b[0] - a[0] * b[2],
    a[0] * b[1] - a[1] * b[0],
  ];
}

export function v3len(a: Vec3): number {
  return Math.sqrt(a[0] * a[0] + a[1] * a[1] + a[2] * a[2]);
}

export function v3normalize(a: Vec3): Vec3 {
  const l = v3len(a);
  if (l < 1e-10) return [0, 0, 0];
  return [a[0] / l, a[1] / l, a[2] / l];
}

export function v3dist(a: Vec3, b: Vec3): number {
  return v3len(v3sub(a, b));
}

export function v3lerp(a: Vec3, b: Vec3, t: number): Vec3 {
  return [
    a[0] + (b[0] - a[0]) * t,
    a[1] + (b[1] - a[1]) * t,
    a[2] + (b[2] - a[2]) * t,
  ];
}

// ─── Mat4 ───────────────────────────────────────────────────────────────────

export function mat4Identity(): Mat4 {
  const m = new Float32Array(16);
  m[0] = m[5] = m[10] = m[15] = 1;
  return m;
}

export function mat4Perspective(fovY: number, aspect: number, near: number, far: number): Mat4 {
  const m = new Float32Array(16);
  const f = 1.0 / Math.tan(fovY / 2);
  m[0] = f / aspect;
  m[5] = f;
  m[10] = (far + near) / (near - far);
  m[11] = -1;
  m[14] = (2 * far * near) / (near - far);
  return m;
}

export function mat4LookAt(eye: Vec3, target: Vec3, up: Vec3): Mat4 {
  const z = v3normalize(v3sub(eye, target));
  const x = v3normalize(v3cross(up, z));
  const y = v3cross(z, x);
  const m = new Float32Array(16);
  m[0] = x[0]; m[1] = y[0]; m[2] = z[0]; m[3] = 0;
  m[4] = x[1]; m[5] = y[1]; m[6] = z[1]; m[7] = 0;
  m[8] = x[2]; m[9] = y[2]; m[10] = z[2]; m[11] = 0;
  m[12] = -v3dot(x, eye);
  m[13] = -v3dot(y, eye);
  m[14] = -v3dot(z, eye);
  m[15] = 1;
  return m;
}

export function mat4Multiply(a: Mat4, b: Mat4): Mat4 {
  const out = new Float32Array(16);
  for (let i = 0; i < 4; i++) {
    for (let j = 0; j < 4; j++) {
      let s = 0;
      for (let k = 0; k < 4; k++) {
        s += a[i + k * 4] * b[k + j * 4];
      }
      out[i + j * 4] = s;
    }
  }
  return out;
}

export function mat4TransformPoint(m: Mat4, p: Vec3): Vec3 {
  const w = m[3] * p[0] + m[7] * p[1] + m[11] * p[2] + m[15];
  return [
    (m[0] * p[0] + m[4] * p[1] + m[8] * p[2] + m[12]) / w,
    (m[1] * p[0] + m[5] * p[1] + m[9] * p[2] + m[13]) / w,
    (m[2] * p[0] + m[6] * p[1] + m[10] * p[2] + m[14]) / w,
  ];
}

export function mat4TransformDir(m: Mat4, d: Vec3): Vec3 {
  return [
    m[0] * d[0] + m[4] * d[1] + m[8] * d[2],
    m[1] * d[0] + m[5] * d[1] + m[9] * d[2],
    m[2] * d[0] + m[6] * d[1] + m[10] * d[2],
  ];
}

export function mat4Translation(x: number, y: number, z: number): Mat4 {
  const m = mat4Identity();
  m[12] = x; m[13] = y; m[14] = z;
  return m;
}

export function mat4Scale(sx: number, sy: number, sz: number): Mat4 {
  const m = new Float32Array(16);
  m[0] = sx; m[5] = sy; m[10] = sz; m[15] = 1;
  return m;
}

export function mat4RotationY(angle: number): Mat4 {
  const m = mat4Identity();
  const c = Math.cos(angle);
  const s = Math.sin(angle);
  m[0] = c; m[8] = s;
  m[2] = -s; m[10] = c;
  return m;
}

export function mat4Inverse(m: Mat4): Mat4 {
  const inv = new Float32Array(16);
  inv[0] = m[5]*m[10]*m[15] - m[5]*m[11]*m[14] - m[9]*m[6]*m[15] + m[9]*m[7]*m[14] + m[13]*m[6]*m[11] - m[13]*m[7]*m[10];
  inv[4] = -m[4]*m[10]*m[15] + m[4]*m[11]*m[14] + m[8]*m[6]*m[15] - m[8]*m[7]*m[14] - m[12]*m[6]*m[11] + m[12]*m[7]*m[10];
  inv[8] = m[4]*m[9]*m[15] - m[4]*m[11]*m[13] - m[8]*m[5]*m[15] + m[8]*m[7]*m[13] + m[12]*m[5]*m[11] - m[12]*m[7]*m[9];
  inv[12] = -m[4]*m[9]*m[14] + m[4]*m[10]*m[13] + m[8]*m[5]*m[14] - m[8]*m[6]*m[13] - m[12]*m[5]*m[10] + m[12]*m[6]*m[9];
  inv[1] = -m[1]*m[10]*m[15] + m[1]*m[11]*m[14] + m[9]*m[2]*m[15] - m[9]*m[3]*m[14] - m[13]*m[2]*m[11] + m[13]*m[3]*m[10];
  inv[5] = m[0]*m[10]*m[15] - m[0]*m[11]*m[14] - m[8]*m[2]*m[15] + m[8]*m[3]*m[14] + m[12]*m[2]*m[11] - m[12]*m[3]*m[10];
  inv[9] = -m[0]*m[9]*m[15] + m[0]*m[11]*m[13] + m[8]*m[1]*m[15] - m[8]*m[3]*m[13] - m[12]*m[1]*m[11] + m[12]*m[3]*m[9];
  inv[13] = m[0]*m[9]*m[14] - m[0]*m[10]*m[13] - m[8]*m[1]*m[14] + m[8]*m[2]*m[13] + m[12]*m[1]*m[10] - m[12]*m[2]*m[9];
  inv[2] = m[1]*m[6]*m[15] - m[1]*m[7]*m[14] - m[5]*m[2]*m[15] + m[5]*m[3]*m[14] + m[13]*m[2]*m[7] - m[13]*m[3]*m[6];
  inv[6] = -m[0]*m[6]*m[15] + m[0]*m[7]*m[14] + m[4]*m[2]*m[15] - m[4]*m[3]*m[14] - m[12]*m[2]*m[7] + m[12]*m[3]*m[6];
  inv[10] = m[0]*m[5]*m[15] - m[0]*m[7]*m[13] - m[4]*m[1]*m[15] + m[4]*m[3]*m[13] + m[12]*m[1]*m[7] - m[12]*m[3]*m[5];
  inv[14] = -m[0]*m[5]*m[14] + m[0]*m[6]*m[13] + m[4]*m[1]*m[14] - m[4]*m[2]*m[13] - m[12]*m[1]*m[6] + m[12]*m[2]*m[5];
  inv[3] = -m[1]*m[6]*m[11] + m[1]*m[7]*m[10] + m[5]*m[2]*m[11] - m[5]*m[3]*m[10] - m[9]*m[2]*m[7] + m[9]*m[3]*m[6];
  inv[7] = m[0]*m[6]*m[11] - m[0]*m[7]*m[10] - m[4]*m[2]*m[11] + m[4]*m[3]*m[10] + m[8]*m[2]*m[7] - m[8]*m[3]*m[6];
  inv[11] = -m[0]*m[5]*m[11] + m[0]*m[7]*m[9] + m[4]*m[1]*m[11] - m[4]*m[3]*m[9] - m[8]*m[1]*m[7] + m[8]*m[3]*m[5];
  inv[15] = m[0]*m[5]*m[10] - m[0]*m[6]*m[9] - m[4]*m[1]*m[10] + m[4]*m[2]*m[9] + m[8]*m[1]*m[6] - m[8]*m[2]*m[5];

  const det = m[0]*inv[0] + m[1]*inv[4] + m[2]*inv[8] + m[3]*inv[12];
  if (Math.abs(det) < 1e-10) return mat4Identity();
  const invDet = 1.0 / det;
  for (let i = 0; i < 16; i++) inv[i] *= invDet;
  return inv;
}

// ─── Frustum ────────────────────────────────────────────────────────────────

/** Six planes [nx,ny,nz,d] where nx*x+ny*y+nz*z+d >= 0 means inside. */
export type Frustum = Vec4[];

export function extractFrustumPlanes(viewProj: Mat4): Frustum {
  const planes: Frustum = [];
  const m = viewProj;
  // Left
  planes.push(normalizePlane([m[3]+m[0], m[7]+m[4], m[11]+m[8], m[15]+m[12]]));
  // Right
  planes.push(normalizePlane([m[3]-m[0], m[7]-m[4], m[11]-m[8], m[15]-m[12]]));
  // Bottom
  planes.push(normalizePlane([m[3]+m[1], m[7]+m[5], m[11]+m[9], m[15]+m[13]]));
  // Top
  planes.push(normalizePlane([m[3]-m[1], m[7]-m[5], m[11]-m[9], m[15]-m[13]]));
  // Near
  planes.push(normalizePlane([m[3]+m[2], m[7]+m[6], m[11]+m[10], m[15]+m[14]]));
  // Far
  planes.push(normalizePlane([m[3]-m[2], m[7]-m[6], m[11]-m[10], m[15]-m[14]]));
  return planes;
}

function normalizePlane(p: Vec4): Vec4 {
  const len = Math.sqrt(p[0]*p[0] + p[1]*p[1] + p[2]*p[2]);
  if (len < 1e-10) return p;
  return [p[0]/len, p[1]/len, p[2]/len, p[3]/len];
}

/** Returns true if sphere is outside the frustum. */
export function sphereOutsideFrustum(
  planes: Frustum,
  cx: number, cy: number, cz: number, r: number,
): boolean {
  for (const p of planes) {
    if (p[0]*cx + p[1]*cy + p[2]*cz + p[3] < -r) return true;
  }
  return false;
}

/** Compute projected screen-space error. */
export function projectedError(
  geometricError: number,
  distance: number,
  screenHeight: number,
  fovY: number,
): number {
  if (distance < 0.001) return 1e6; // very close = always refine
  const projScale = screenHeight / (2.0 * Math.tan(fovY / 2));
  return (geometricError * projScale) / distance;
}
