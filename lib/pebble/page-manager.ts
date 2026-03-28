/**
 * Software page residency manager.
 *
 * Since WebGPU doesn't expose sparse/tiled resources, we manage
 * geometry page residency in software:
 *   - A fixed-size GPU buffer pool for vertex/index data
 *   - A page table mapping pageId → resident slot (or -1)
 *   - Upload budget per frame
 *   - LRU eviction when pool is full
 *   - Parent fallback when child pages aren't resident
 */

import type { Page } from './types';
import { DEFAULT_MAX_RESIDENT_PAGES, DEFAULT_UPLOAD_BUDGET_BYTES } from './constants';

export interface PageSlot {
  pageId: number;
  lastUsedFrame: number;
  resident: boolean;
}

export interface PageRequest {
  pageId: number;
  priority: number; // higher = more urgent
}

export class PageManager {
  /** Max number of pages that can be resident simultaneously */
  readonly maxResidentPages: number;

  /** Maps pageId → slot index (-1 if not resident) */
  private pageTable: Map<number, number>;

  /** Slot pool */
  private slots: PageSlot[];

  /** All known pages */
  private pages: Map<number, Page>;

  /** Current frame number */
  private frameCounter: number;

  /** Pending upload requests */
  private pendingRequests: PageRequest[];

  /** Max bytes to upload per frame */
  readonly uploadBudgetBytes: number;

  constructor(maxResidentPages: number = DEFAULT_MAX_RESIDENT_PAGES, uploadBudgetBytes: number = DEFAULT_UPLOAD_BUDGET_BYTES) {
    this.maxResidentPages = maxResidentPages;
    this.uploadBudgetBytes = uploadBudgetBytes;
    this.pageTable = new Map();
    this.slots = [];
    this.pages = new Map();
    this.frameCounter = 0;
    this.pendingRequests = [];

    for (let i = 0; i < maxResidentPages; i++) {
      this.slots.push({ pageId: -1, lastUsedFrame: -1, resident: false });
    }
  }

  /** Register all pages from the scene. */
  registerPages(pages: Page[]): void {
    for (const p of pages) {
      this.pages.set(p.id, p);
    }
  }

  /** Check if a page is currently resident. */
  isResident(pageId: number): boolean {
    const slot = this.pageTable.get(pageId);
    return slot !== undefined && this.slots[slot].resident;
  }

  /** Touch a page (mark as used this frame). */
  touchPage(pageId: number): void {
    const slot = this.pageTable.get(pageId);
    if (slot !== undefined) {
      this.slots[slot].lastUsedFrame = this.frameCounter;
    }
  }

  /** Request a page to be made resident. */
  requestPage(pageId: number, priority: number): void {
    if (this.isResident(pageId)) {
      this.touchPage(pageId);
      return;
    }
    // Avoid duplicate requests
    if (!this.pendingRequests.find(r => r.pageId === pageId)) {
      this.pendingRequests.push({ pageId, priority });
    }
  }

  /** Find a free slot or evict the LRU page. Returns slot index. */
  private findSlot(): number {
    // First: find an empty slot
    for (let i = 0; i < this.slots.length; i++) {
      if (!this.slots[i].resident) return i;
    }

    // Evict LRU
    let lruSlot = 0;
    let lruFrame = Infinity;
    for (let i = 0; i < this.slots.length; i++) {
      if (this.slots[i].lastUsedFrame < lruFrame) {
        lruFrame = this.slots[i].lastUsedFrame;
        lruSlot = i;
      }
    }

    // Evict
    const evictedPageId = this.slots[lruSlot].pageId;
    this.pageTable.delete(evictedPageId);
    this.slots[lruSlot].resident = false;
    this.slots[lruSlot].pageId = -1;

    return lruSlot;
  }

  /**
   * Process pending page requests. Returns list of pages that were made resident.
   * Call once per frame.
   */
  processRequests(): number[] {
    this.frameCounter++;

    // Sort by priority (highest first)
    this.pendingRequests.sort((a, b) => b.priority - a.priority);

    const uploaded: number[] = [];
    let bytesUsed = 0;

    for (const req of this.pendingRequests) {
      if (this.isResident(req.pageId)) continue;

      const page = this.pages.get(req.pageId);
      if (!page) continue;

      const pageSize = page.vertexBufferSize + page.indexBufferSize;
      if (bytesUsed + pageSize > this.uploadBudgetBytes && uploaded.length > 0) break;

      const slot = this.findSlot();
      this.slots[slot] = {
        pageId: req.pageId,
        lastUsedFrame: this.frameCounter,
        resident: true,
      };
      this.pageTable.set(req.pageId, slot);
      bytesUsed += pageSize;
      uploaded.push(req.pageId);
    }

    this.pendingRequests = [];
    return uploaded;
  }

  /** Make all pages resident immediately (for small scenes). */
  makeAllResident(): void {
    for (const [pageId] of this.pages) {
      if (!this.isResident(pageId)) {
        const slot = this.findSlot();
        this.slots[slot] = {
          pageId,
          lastUsedFrame: this.frameCounter,
          resident: true,
        };
        this.pageTable.set(pageId, slot);
      }
    }
  }

  /** Get residency stats. */
  getStats(): { resident: number; total: number } {
    let resident = 0;
    for (const slot of this.slots) {
      if (slot.resident) resident++;
    }
    return { resident, total: this.pages.size };
  }

  /** Reset all state. */
  reset(): void {
    this.pageTable.clear();
    this.pendingRequests = [];
    this.frameCounter = 0;
    for (let i = 0; i < this.slots.length; i++) {
      this.slots[i] = { pageId: -1, lastUsedFrame: -1, resident: false };
    }
  }
}
