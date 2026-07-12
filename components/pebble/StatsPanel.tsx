'use client';

import type { FrameStats } from '@/lib/pebble/types';

interface StatsPanelProps {
  stats: FrameStats;
}

export function StatsPanel({ stats }: StatsPanelProps) {
  const pct = stats.totalTriangles > 0
    ? ((stats.renderedTriangles / stats.totalTriangles) * 100).toFixed(1)
    : '0.0';

  return (
    <div className="absolute top-4 left-4 bg-black/80 text-green-400 font-mono text-xs p-3 rounded-lg space-y-1 pointer-events-none select-none min-w-[220px]">
      <div className="text-green-300 font-bold text-sm mb-2">Pebble Stats</div>
      <Row label="FPS" value={`${stats.fps}`} />
      <Row label="Frame" value={`${stats.frameTimeMs.toFixed(1)} ms`} />
      <Row label="GPU" value={`${stats.gpuTimeMs.toFixed(2)} ms`} />
      <div className="border-t border-green-900 my-1" />
      <Row label="Clusters" value={`${stats.visibleClusters} / ${stats.totalClusters}`} />
      <Row label="Triangles" value={`${fmt(stats.renderedTriangles)} / ${fmt(stats.totalTriangles)}`} />
      <Row label="Rendered" value={`${pct}%`} />
      <div className="border-t border-green-900 my-1" />
      <Row label="Instances" value={`${stats.visibleInstances} / ${stats.totalInstances}`} />
      <Row label="Pages" value={`${stats.residentPages} / ${stats.totalPages}`} />
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-4">
      <span className="text-green-600">{label}</span>
      <span>{value}</span>
    </div>
  );
}

function fmt(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return `${n}`;
}
