import { describe, it, expect } from 'vitest';
import * as C from '../constants';

describe('constants', () => {
  it('cluster triangles: target < max', () => {
    expect(C.TARGET_CLUSTER_TRIANGLES).toBeLessThan(C.MAX_CLUSTER_TRIANGLES);
  });

  it('GPU_CLUSTER_BYTES = GPU_CLUSTER_U32_STRIDE * 4', () => {
    expect(C.GPU_CLUSTER_BYTES).toBe(C.GPU_CLUSTER_U32_STRIDE * 4);
  });

  it('GPU_INSTANCE_BYTES = GPU_INSTANCE_U32_STRIDE * 4', () => {
    expect(C.GPU_INSTANCE_BYTES).toBe(C.GPU_INSTANCE_U32_STRIDE * 4);
  });

  it('PARENT_ERROR_SCALE > 1', () => {
    expect(C.PARENT_ERROR_SCALE).toBeGreaterThan(1);
  });

  it('VERTEX_STRIDE_FLOATS = 8 (pos3 + normal3 + uv2)', () => {
    expect(C.VERTEX_STRIDE_FLOATS).toBe(8);
  });

  it('INVALID_INDEX is max u32', () => {
    expect(C.INVALID_INDEX).toBe(0xFFFFFFFF);
  });

  it('upload budget is positive', () => {
    expect(C.DEFAULT_UPLOAD_BUDGET_BYTES).toBeGreaterThan(0);
  });

  it('max resident pages is positive', () => {
    expect(C.DEFAULT_MAX_RESIDENT_PAGES).toBeGreaterThan(0);
  });

  it('zoom limits are ordered', () => {
    expect(C.MIN_ZOOM_DISTANCE).toBeLessThan(C.MAX_ZOOM_DISTANCE);
  });

  it('clear color has valid RGBA', () => {
    expect(C.CLEAR_COLOR.r).toBeGreaterThanOrEqual(0);
    expect(C.CLEAR_COLOR.g).toBeGreaterThanOrEqual(0);
    expect(C.CLEAR_COLOR.b).toBeGreaterThanOrEqual(0);
    expect(C.CLEAR_COLOR.a).toBe(1);
  });
});
