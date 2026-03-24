'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import type { Camera, FrameStats, RenderSettings, BuiltScene } from '@/lib/pebble/types';
import { DEFAULT_RENDER_SETTINGS } from '@/lib/pebble/types';
import { StatsPanel } from './StatsPanel';
import { ControlsPanel } from './ControlsPanel';

// Dynamic imports to avoid SSR issues with WebGPU
async function loadPebble() {
  const { PebbleRenderer, buildScene, generateSphere, generateTerrain, generateTorus, generateTrefoilKnot, generateOcean, generateTree, generateMountains, generateMassiveScene, mat4Identity, mat4Translation, mat4RotationY, mat4Multiply, mat4Scale } = await import('@/lib/pebble');
  return { PebbleRenderer, buildScene, generateSphere, generateTerrain, generateTorus, generateTrefoilKnot, generateOcean, generateTree, generateMountains, generateMassiveScene, mat4Identity, mat4Translation, mat4RotationY, mat4Multiply, mat4Scale };
}

function createScene(sceneId: string, modules: Awaited<ReturnType<typeof loadPebble>>): BuiltScene {
  const { buildScene, generateSphere, generateTerrain, generateTorus, generateTrefoilKnot, generateOcean, generateTree, generateMountains, generateMassiveScene, mat4Identity, mat4Translation, mat4RotationY, mat4Multiply, mat4Scale } = modules;

  switch (sceneId) {
    case 'sphere':
      return buildScene([{
        name: 'sphere',
        raw: generateSphere(2, 16, 32),
        instances: [mat4Identity()],
      }]);

    case 'sphere-hd':
      return buildScene([{
        name: 'sphere-hd',
        raw: generateSphere(2, 64, 128),
        instances: [mat4Identity()],
      }]);

    case 'terrain':
      return buildScene([{
        name: 'terrain',
        raw: generateTerrain(20, 20, 128, 128, 3, 0.5),
        instances: [mat4Identity()],
      }]);

    case 'torus':
      return buildScene([{
        name: 'torus',
        raw: generateTorus(2, 0.7, 48, 24),
        instances: [mat4Identity()],
      }]);

    case 'multi': {
      const instances = [];
      for (let x = 0; x < 5; x++) {
        for (let z = 0; z < 5; z++) {
          instances.push(mat4Translation((x - 2) * 3, 0, (z - 2) * 3));
        }
      }
      return buildScene([{
        name: 'spheres',
        raw: generateSphere(1, 16, 32),
        instances,
      }]);
    }

    case 'massive':
      return buildScene([{
        name: 'massive',
        raw: generateMassiveScene(5, 16),
        instances: [mat4Identity()],
      }]);

    case 'knot':
      return buildScene([{
        name: 'knot',
        raw: generateTrefoilKnot(0.3, 2, 256, 32),
        instances: [mat4Identity()],
      }]);

    // Many torus instances scattered at varying distances — LOD transitions are
    // clearly visible as you zoom in/out: distant rings collapse to low-detail
    // clusters while close ones stay fully detailed.
    case 'lod-field': {
      const instances = [];
      const rings = [
        { count: 1,  radius: 0,  y: 0 },
        { count: 6,  radius: 5,  y: 0 },
        { count: 12, radius: 11, y: 0.5 },
        { count: 18, radius: 18, y: 1.5 },
        { count: 24, radius: 26, y: 3 },
      ];
      for (const ring of rings) {
        for (let i = 0; i < ring.count; i++) {
          const angle = (i / Math.max(ring.count, 1)) * Math.PI * 2;
          const x = Math.cos(angle) * ring.radius;
          const z = Math.sin(angle) * ring.radius;
          instances.push(mat4Multiply(
            mat4Translation(x, ring.y, z),
            mat4RotationY(angle),
          ));
        }
      }
      return buildScene([{
        name: 'torus',
        raw: generateTorus(1, 0.35, 48, 24),
        instances,
      }]);
    }

    // High-detail terrain + a grid of spheres above it at varying heights.
    // Tests LOD on both a large continuous surface and many discrete objects.
    case 'landscape': {
      const sphereInstances = [];
      for (let x = -3; x <= 3; x++) {
        for (let z = -3; z <= 3; z++) {
          const h = Math.sin(x * 0.8) * Math.cos(z * 0.8) * 2 + 3;
          sphereInstances.push(mat4Multiply(
            mat4Translation(x * 5, h, z * 5),
            mat4Scale(0.6, 0.6, 0.6),
          ));
        }
      }
      return buildScene([
        {
          name: 'terrain',
          raw: generateTerrain(60, 60, 256, 256, 4, 0.3),
          instances: [mat4Identity()],
        },
        {
          name: 'sphere',
          raw: generateSphere(1, 32, 64),
          instances: sphereInstances,
        },
      ]);
    }

    // Ocean: high-res Gerstner wave surface — tests LOD on a large uniform mesh
    case 'ocean':
      return buildScene([{
        name: 'ocean',
        raw: generateOcean(120, 256, 1.5),
        instances: [mat4Identity()],
      }]);

    // Forest: a terrain base with hundreds of tree instances scattered across it.
    // LOD effect is dramatic — zoom out to see trees collapse to single clusters.
    case 'forest': {
      const treeInstances = [];
      const rng = { v: 42 };
      const rand = () => { rng.v = (rng.v * 1664525 + 1013904223) & 0xffffffff; return (rng.v >>> 0) / 0xffffffff; };
      for (let i = 0; i < 200; i++) {
        const angle = rand() * Math.PI * 2;
        const r = rand() * 35 + 3;
        const x = Math.cos(angle) * r;
        const z = Math.sin(angle) * r;
        const scale = 0.6 + rand() * 0.8;
        treeInstances.push(mat4Multiply(
          mat4Translation(x, 0, z),
          mat4Multiply(mat4RotationY(rand() * Math.PI * 2), mat4Scale(scale, scale, scale)),
        ));
      }
      return buildScene([
        {
          name: 'ground',
          raw: generateTerrain(80, 80, 192, 192, 1.5, 0.2),
          instances: [mat4Identity()],
        },
        {
          name: 'tree',
          raw: generateTree(0.15, 1.2, 0.9, 3.5, 12, 3),
          instances: treeInstances,
        },
      ]);
    }

    // Mountain vista: dramatic rocky peaks with snow-cap spheres and distant rings.
    case 'mountains': {
      const decorInstances = [];
      for (let i = 0; i < 30; i++) {
        const angle = (i / 30) * Math.PI * 2;
        const r = 28 + Math.sin(i * 7.3) * 8;
        decorInstances.push(mat4Multiply(
          mat4Translation(Math.cos(angle) * r, 2, Math.sin(angle) * r),
          mat4Scale(0.5, 0.5, 0.5),
        ));
      }
      return buildScene([
        {
          name: 'mountains',
          raw: generateMountains(80, 192, 22),
          instances: [mat4Identity()],
        },
        {
          name: 'rock',
          raw: generateSphere(1, 16, 32),
          instances: decorInstances,
        },
      ]);
    }

    default:
      return buildScene([{
        name: 'sphere',
        raw: generateSphere(2, 16, 32),
        instances: [mat4Identity()],
      }]);
  }
}

export function PebbleViewer() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const rendererRef = useRef<InstanceType<typeof import('@/lib/pebble').PebbleRenderer> | null>(null);
  const animRef = useRef<number>(0);
  const modulesRef = useRef<Awaited<ReturnType<typeof loadPebble>> | null>(null);

  const [stats, setStats] = useState<FrameStats>({
    fps: 0, frameTimeMs: 0,
    totalClusters: 0, visibleClusters: 0,
    totalTriangles: 0, renderedTriangles: 0,
    totalInstances: 0, visibleInstances: 0,
    residentPages: 0, totalPages: 0,
    gpuTimeMs: 0,
  });
  const [settings, setSettings] = useState<RenderSettings>(DEFAULT_RENDER_SETTINGS);
  const [scene, setScene] = useState('sphere');
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  // Camera state
  const cameraRef = useRef({
    theta: 0.5,       // azimuth
    phi: 0.4,         // elevation
    distance: 8,      // from target
    targetX: 0,
    targetY: 0,
    targetZ: 0,
  });
  const settingsRef = useRef(settings);
  settingsRef.current = settings;

  const getCamera = useCallback((): Camera => {
    const cam = cameraRef.current;
    const x = cam.distance * Math.sin(cam.phi) * Math.cos(cam.theta) + cam.targetX;
    const y = cam.distance * Math.cos(cam.phi) + cam.targetY;
    const z = cam.distance * Math.sin(cam.phi) * Math.sin(cam.theta) + cam.targetZ;
    const canvas = canvasRef.current!;
    return {
      position: new Float32Array([x, y, z]),
      target: new Float32Array([cam.targetX, cam.targetY, cam.targetZ]),
      up: new Float32Array([0, 1, 0]),
      fovY: Math.PI / 3,
      aspect: canvas.width / canvas.height,
      near: 0.1,
      far: 500,
    };
  }, []);

  // Initialize renderer
  useEffect(() => {
    let cancelled = false;

    async function init() {
      const canvas = canvasRef.current;
      if (!canvas) return;

      try {
        const modules = await loadPebble();
        modulesRef.current = modules;

        const renderer = new modules.PebbleRenderer();
        const ok = await renderer.init(canvas);
        if (!ok) {
          setError('WebGPU is not available in this browser. Try Chrome 113+ or Edge 113+.');
          setLoading(false);
          return;
        }
        if (cancelled) { renderer.destroy(); return; }

        rendererRef.current = renderer;

        // Build initial scene
        const builtScene = createScene(scene, modules);
        renderer.loadScene(builtScene);

        setLoading(false);

        // Start render loop
        function frame() {
          if (cancelled) return;
          const camera = getCamera();
          renderer.render(camera, settingsRef.current);
          const s = renderer.getStats();
          // Estimate rendered triangles from visible clusters
          setStats(prev => ({
            ...s,
            visibleClusters: s.totalClusters, // Will be refined with GPU readback
            renderedTriangles: s.totalTriangles, // Will be refined
            visibleInstances: s.totalInstances,
          }));
          animRef.current = requestAnimationFrame(frame);
        }
        animRef.current = requestAnimationFrame(frame);

      } catch (err: any) {
        setError(err.message || 'Failed to initialize WebGPU');
        setLoading(false);
      }
    }

    init();
    return () => {
      cancelled = true;
      cancelAnimationFrame(animRef.current);
      rendererRef.current?.destroy();
      rendererRef.current = null;
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Handle scene changes
  useEffect(() => {
    if (!rendererRef.current || !modulesRef.current) return;
    try {
      const builtScene = createScene(scene, modulesRef.current);
      rendererRef.current.loadScene(builtScene);
    } catch (err: any) {
      setError(err.message);
    }
  }, [scene]);

  // Handle resize
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const observer = new ResizeObserver(entries => {
      for (const entry of entries) {
        const { width, height } = entry.contentRect;
        const dpr = window.devicePixelRatio || 1;
        canvas.width = Math.floor(width * dpr);
        canvas.height = Math.floor(height * dpr);
        rendererRef.current?.resize(canvas.width, canvas.height);
      }
    });
    observer.observe(canvas);
    return () => observer.disconnect();
  }, []);

  // Mouse controls: orbit + zoom + pan
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    let isDragging = false;
    let isPanning = false;
    let lastX = 0, lastY = 0;

    const onMouseDown = (e: MouseEvent) => {
      isDragging = true;
      isPanning = e.button === 2;
      lastX = e.clientX;
      lastY = e.clientY;
    };

    const onMouseMove = (e: MouseEvent) => {
      if (!isDragging) return;
      const dx = e.clientX - lastX;
      const dy = e.clientY - lastY;
      lastX = e.clientX;
      lastY = e.clientY;

      const cam = cameraRef.current;
      if (isPanning) {
        const panSpeed = cam.distance * 0.002;
        cam.targetX -= dx * panSpeed;
        cam.targetY += dy * panSpeed;
      } else {
        cam.theta -= dx * 0.005;
        cam.phi = Math.max(0.1, Math.min(Math.PI - 0.1, cam.phi + dy * 0.005));
      }
    };

    const onMouseUp = () => { isDragging = false; isPanning = false; };

    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const cam = cameraRef.current;
      cam.distance *= e.deltaY > 0 ? 1.1 : 0.9;
      cam.distance = Math.max(1, Math.min(200, cam.distance));
    };

    const onContextMenu = (e: MouseEvent) => e.preventDefault();

    canvas.addEventListener('mousedown', onMouseDown);
    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
    canvas.addEventListener('wheel', onWheel, { passive: false });
    canvas.addEventListener('contextmenu', onContextMenu);

    // Touch controls
    let touchStartDist = 0;
    let touchStartX = 0, touchStartY = 0;

    const onTouchStart = (e: TouchEvent) => {
      if (e.touches.length === 1) {
        isDragging = true;
        lastX = e.touches[0].clientX;
        lastY = e.touches[0].clientY;
      } else if (e.touches.length === 2) {
        const dx = e.touches[0].clientX - e.touches[1].clientX;
        const dy = e.touches[0].clientY - e.touches[1].clientY;
        touchStartDist = Math.sqrt(dx * dx + dy * dy);
      }
    };

    const onTouchMove = (e: TouchEvent) => {
      e.preventDefault();
      if (e.touches.length === 1 && isDragging) {
        const dx = e.touches[0].clientX - lastX;
        const dy = e.touches[0].clientY - lastY;
        lastX = e.touches[0].clientX;
        lastY = e.touches[0].clientY;
        const cam = cameraRef.current;
        cam.theta -= dx * 0.005;
        cam.phi = Math.max(0.1, Math.min(Math.PI - 0.1, cam.phi + dy * 0.005));
      } else if (e.touches.length === 2) {
        const dx = e.touches[0].clientX - e.touches[1].clientX;
        const dy = e.touches[0].clientY - e.touches[1].clientY;
        const dist = Math.sqrt(dx * dx + dy * dy);
        const cam = cameraRef.current;
        cam.distance *= touchStartDist / dist;
        cam.distance = Math.max(1, Math.min(200, cam.distance));
        touchStartDist = dist;
      }
    };

    const onTouchEnd = () => { isDragging = false; };

    canvas.addEventListener('touchstart', onTouchStart, { passive: false });
    canvas.addEventListener('touchmove', onTouchMove, { passive: false });
    canvas.addEventListener('touchend', onTouchEnd);

    return () => {
      canvas.removeEventListener('mousedown', onMouseDown);
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
      canvas.removeEventListener('wheel', onWheel);
      canvas.removeEventListener('contextmenu', onContextMenu);
      canvas.removeEventListener('touchstart', onTouchStart);
      canvas.removeEventListener('touchmove', onTouchMove);
      canvas.removeEventListener('touchend', onTouchEnd);
    };
  }, []);

  if (error) {
    return (
      <div className="flex items-center justify-center h-screen bg-stone-950 text-red-400 font-mono p-8">
        <div className="text-center space-y-4 max-w-md">
          <div className="text-2xl">WebGPU Not Available</div>
          <div className="text-sm text-stone-400">{error}</div>
          <div className="text-xs text-stone-600">
            WebGPU requires Chrome 113+, Edge 113+, or Firefox Nightly with flags enabled.
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="relative w-full h-screen bg-stone-950">
      {loading && (
        <div className="absolute inset-0 flex items-center justify-center z-10">
          <div className="text-stone-400 font-mono text-sm animate-pulse">
            Building cluster hierarchy...
          </div>
        </div>
      )}
      <canvas
        ref={canvasRef}
        className="w-full h-full"
        style={{ display: loading ? 'none' : 'block' }}
      />
      {!loading && (
        <>
          <StatsPanel stats={stats} />
          <ControlsPanel
            settings={settings}
            onChange={setSettings}
            scene={scene}
            onSceneChange={setScene}
          />
          <div className="absolute bottom-4 left-1/2 -translate-x-1/2 text-stone-600 font-mono text-[10px] pointer-events-none">
            Pebble -- WebGPU Virtualized Geometry
          </div>
        </>
      )}
    </div>
  );
}
