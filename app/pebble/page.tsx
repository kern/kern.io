import type { Metadata } from 'next';
import { PebbleViewer } from '@/components/pebble/PebbleViewer';

export const metadata: Metadata = {
  title: 'Pebble — WebGPU Virtualized Geometry',
  description: 'Virtualized geometry renderer built on WebGPU with hierarchical cluster LOD, GPU-driven culling, and vertex pulling.',
};

export default function PebblePage() {
  return <PebbleViewer />;
}
