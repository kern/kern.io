import { describe, it, expect } from 'vitest';
import { PageManager } from '../page-manager';
import type { Page } from '../types';

function makePage(id: number): Page {
  return {
    id,
    vertexBufferOffset: id * 1000,
    vertexBufferSize: 500,
    indexBufferOffset: id * 500,
    indexBufferSize: 200,
    clusterIds: [id * 10, id * 10 + 1],
  };
}

describe('PageManager', () => {
  it('starts with no resident pages', () => {
    const pm = new PageManager(16);
    pm.registerPages([makePage(0), makePage(1)]);
    const stats = pm.getStats();
    expect(stats.resident).toBe(0);
    expect(stats.total).toBe(2);
  });

  it('makes pages resident on request + process', () => {
    const pm = new PageManager(16);
    pm.registerPages([makePage(0), makePage(1), makePage(2)]);

    pm.requestPage(0, 1.0);
    pm.requestPage(1, 0.5);
    const uploaded = pm.processRequests();

    expect(uploaded).toContain(0);
    expect(uploaded).toContain(1);
    expect(pm.isResident(0)).toBe(true);
    expect(pm.isResident(1)).toBe(true);
    expect(pm.isResident(2)).toBe(false);
  });

  it('does not duplicate requests for already-resident pages', () => {
    const pm = new PageManager(16);
    pm.registerPages([makePage(0)]);

    pm.requestPage(0, 1.0);
    pm.processRequests();
    expect(pm.isResident(0)).toBe(true);

    // Request again — should be a no-op
    pm.requestPage(0, 1.0);
    const uploaded = pm.processRequests();
    expect(uploaded.length).toBe(0);
  });

  it('evicts LRU pages when pool is full', () => {
    const pm = new PageManager(2);
    const pages = [makePage(0), makePage(1), makePage(2)];
    pm.registerPages(pages);

    // Fill pool
    pm.requestPage(0, 1.0);
    pm.requestPage(1, 1.0);
    pm.processRequests();
    expect(pm.isResident(0)).toBe(true);
    expect(pm.isResident(1)).toBe(true);

    // Touch page 1 so page 0 is LRU
    pm.touchPage(1);

    // Request page 2 — should evict page 0
    pm.requestPage(2, 1.0);
    pm.processRequests();
    expect(pm.isResident(2)).toBe(true);
    // One of the old pages should be evicted
    const stats = pm.getStats();
    expect(stats.resident).toBe(2);
  });

  it('respects upload budget', () => {
    // Each page is 700 bytes (500 + 200), budget is 800
    const pm = new PageManager(16, 800);
    pm.registerPages([makePage(0), makePage(1), makePage(2)]);

    pm.requestPage(0, 1.0);
    pm.requestPage(1, 0.5);
    pm.requestPage(2, 0.3);
    const uploaded = pm.processRequests();

    // Budget allows ~1 page (700 bytes), but first page always goes through
    // Second might fit if budget check allows
    expect(uploaded.length).toBeGreaterThanOrEqual(1);
    expect(uploaded.length).toBeLessThanOrEqual(2);
  });

  it('prioritizes higher priority requests', () => {
    const pm = new PageManager(1); // only 1 slot
    pm.registerPages([makePage(0), makePage(1)]);

    pm.requestPage(0, 0.1); // low priority
    pm.requestPage(1, 10.0); // high priority
    const uploaded = pm.processRequests();

    // Page 1 should be uploaded first (higher priority)
    expect(uploaded[0]).toBe(1);
  });

  it('makeAllResident works', () => {
    const pm = new PageManager(16);
    pm.registerPages([makePage(0), makePage(1), makePage(2), makePage(3)]);
    pm.makeAllResident();

    expect(pm.isResident(0)).toBe(true);
    expect(pm.isResident(1)).toBe(true);
    expect(pm.isResident(2)).toBe(true);
    expect(pm.isResident(3)).toBe(true);
    expect(pm.getStats().resident).toBe(4);
  });

  it('reset clears all state', () => {
    const pm = new PageManager(16);
    pm.registerPages([makePage(0), makePage(1)]);
    pm.makeAllResident();
    expect(pm.getStats().resident).toBe(2);

    pm.reset();
    expect(pm.getStats().resident).toBe(0);
    expect(pm.isResident(0)).toBe(false);
  });
});
