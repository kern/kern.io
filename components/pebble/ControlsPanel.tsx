'use client';

import type { RenderSettings } from '@/lib/pebble/types';

interface ControlsPanelProps {
  settings: RenderSettings;
  onChange: (settings: RenderSettings) => void;
  scene: string;
  onSceneChange: (scene: string) => void;
}

const SCENES = [
  { id: 'sphere', label: 'Sphere (2K tris)' },
  { id: 'sphere-hd', label: 'HD Sphere (32K tris)' },
  { id: 'terrain', label: 'Terrain (32K tris)' },
  { id: 'torus', label: 'Torus (4.6K tris)' },
  { id: 'knot', label: 'Trefoil Knot (16K tris)' },
  { id: 'multi', label: 'Multi-Object (25 spheres)' },
  { id: 'lod-field', label: 'LOD Field (61 toruses)' },
  { id: 'landscape', label: 'Landscape + Spheres (500K+ tris)' },
  { id: 'ocean', label: 'Ocean (130K tris)' },
  { id: 'forest', label: 'Forest — 200 trees (800K+ tris)' },
  { id: 'mountains', label: 'Mountains (700K+ tris)' },
  { id: 'massive', label: 'Massive (250K+ tris)' },
];

export function ControlsPanel({ settings, onChange, scene, onSceneChange }: ControlsPanelProps) {
  const update = (partial: Partial<RenderSettings>) => {
    onChange({ ...settings, ...partial });
  };

  return (
    <div className="absolute top-4 right-4 bg-black/80 text-stone-300 font-mono text-xs p-3 rounded-lg space-y-3 min-w-[240px]">
      <div className="text-stone-100 font-bold text-sm">Controls</div>

      {/* Scene selector */}
      <div>
        <label className="block text-stone-500 mb-1">Scene</label>
        <select
          value={scene}
          onChange={e => onSceneChange(e.target.value)}
          className="w-full bg-stone-900 text-stone-200 border border-stone-700 rounded px-2 py-1 text-xs"
        >
          {SCENES.map(s => (
            <option key={s.id} value={s.id}>{s.label}</option>
          ))}
        </select>
      </div>

      {/* LOD threshold */}
      <div>
        <label className="block text-stone-500 mb-1">
          LOD Threshold: {settings.lodErrorThreshold.toFixed(2)}
        </label>
        <input
          type="range"
          min="0.1"
          max="10"
          step="0.1"
          value={settings.lodErrorThreshold}
          onChange={e => update({ lodErrorThreshold: parseFloat(e.target.value) })}
          className="w-full accent-green-500"
        />
      </div>

      {/* Toggles */}
      <Toggle
        label="Debug LOD Colors"
        value={settings.debugLODColors}
        onChange={v => update({ debugLODColors: v })}
      />
      <Toggle
        label="Wireframe"
        value={settings.showWireframe}
        onChange={v => update({ showWireframe: v })}
      />
      <Toggle
        label="Frustum Culling"
        value={settings.enableFrustumCulling}
        onChange={v => update({ enableFrustumCulling: v })}
      />
      <Toggle
        label="Backface Culling"
        value={settings.enableBackfaceCulling}
        onChange={v => update({ enableBackfaceCulling: v })}
      />
      <Toggle
        label="Occlusion Culling (HZB)"
        value={settings.enableOcclusionCulling}
        onChange={v => update({ enableOcclusionCulling: v })}
      />
      <Toggle
        label="Freeze Culling"
        value={settings.freezeCulling}
        onChange={v => update({ freezeCulling: v })}
      />

      <div className="border-t border-stone-700 pt-2 text-stone-500 text-[10px] leading-relaxed">
        <div>Drag: orbit | Scroll: zoom</div>
        <div>Right-drag: pan</div>
      </div>
    </div>
  );
}

function Toggle({
  label, value, onChange,
}: {
  label: string; value: boolean; onChange: (v: boolean) => void;
}) {
  return (
    <label className="flex items-center gap-2 cursor-pointer">
      <input
        type="checkbox"
        checked={value}
        onChange={e => onChange(e.target.checked)}
        className="accent-green-500"
      />
      <span>{label}</span>
    </label>
  );
}
