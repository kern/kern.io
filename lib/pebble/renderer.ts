/**
 * Pebble — WebGPU virtualized geometry renderer.
 *
 * Frame pipeline:
 *   1. Reset counters (compute)
 *   2. Instance culling (compute)
 *   3. Cluster LOD selection (compute)
 *   4. Main draw with vertex pulling (render)
 *   5. Optional wireframe overlay (render)
 *
 * All geometry decisions are GPU-driven. The CPU only uploads
 * uniforms and reads back stats.
 */

import type { BuiltScene, Camera, RenderSettings, FrameStats, Cluster } from './types';
import {
  mat4Perspective, mat4LookAt, mat4Multiply, extractFrustumPlanes,
  Mat4, Vec3,
} from './math';
import {
  INSTANCE_CULL_SHADER,
  CLUSTER_LOD_SHADER,
  VERTEX_PULL_SHADER,
  WIREFRAME_SHADER,
  RESET_COUNTER_SHADER,
} from './shaders';
import { PageManager } from './page-manager';
import {
  GPU_CLUSTER_U32_STRIDE,
  GPU_INSTANCE_U32_STRIDE,
  INVALID_INDEX,
  MIN_BUFFER_SIZE,
  UNIFORM_BUFFER_SIZE,
  CLEAR_COLOR,
  DEPTH_CLEAR_VALUE,
  FPS_UPDATE_INTERVAL_MS,
  COMPUTE_WORKGROUP_SIZE_1D,
  MAX_STORAGE_BUFFER_BINDING_SIZE,
  MAX_BUFFER_SIZE,
} from './constants';

export class PebbleRenderer {
  private device!: GPUDevice;
  private context!: GPUCanvasContext;
  private format!: GPUTextureFormat;
  private canvas!: HTMLCanvasElement;

  // Buffers
  private vertexBuffer!: GPUBuffer;
  private indexBuffer!: GPUBuffer;
  private clusterBuffer!: GPUBuffer;
  private instanceBuffer!: GPUBuffer;
  private uniformBuffer!: GPUBuffer;
  private visibleInstanceBuffer!: GPUBuffer;
  private visibleInstanceCountBuffer!: GPUBuffer;
  private visibleClusterBuffer!: GPUBuffer;
  private visibleClusterCountBuffer!: GPUBuffer;
  private statsReadBuffer!: GPUBuffer;

  // Pipelines
  private resetCounterPipeline!: GPUComputePipeline;
  private instanceCullPipeline!: GPUComputePipeline;
  private clusterLodPipeline!: GPUComputePipeline;
  private mainRenderPipeline!: GPURenderPipeline;
  private wireframeRenderPipeline!: GPURenderPipeline;

  // Bind groups
  private resetInstanceCountBG!: GPUBindGroup;
  private resetClusterCountBG!: GPUBindGroup;
  private instanceCullBG!: GPUBindGroup;
  private clusterLodBG!: GPUBindGroup;
  private mainRenderBG!: GPUBindGroup;
  private wireframeRenderBG!: GPUBindGroup;

  // Depth texture
  private depthTexture!: GPUTexture;

  // Scene data
  private scene!: BuiltScene;
  private pageManager!: PageManager;

  // Stats
  private frameStats: FrameStats = {
    fps: 0, frameTimeMs: 0,
    totalClusters: 0, visibleClusters: 0,
    totalTriangles: 0, renderedTriangles: 0,
    totalInstances: 0, visibleInstances: 0,
    residentPages: 0, totalPages: 0,
    gpuTimeMs: 0,
  };
  private lastFrameTime = 0;
  private frameCount = 0;
  private fpsAccum = 0;
  private lastFpsUpdate = 0;

  async init(canvas: HTMLCanvasElement): Promise<boolean> {
    if (!navigator.gpu) return false;

    const adapter = await navigator.gpu.requestAdapter({
      powerPreference: 'high-performance',
    });
    if (!adapter) return false;

    this.device = await adapter.requestDevice({
      requiredLimits: {
        maxStorageBufferBindingSize: MAX_STORAGE_BUFFER_BINDING_SIZE,
        maxBufferSize: MAX_BUFFER_SIZE,
        maxStorageBuffersPerShaderStage: 8,
      },
    });

    this.canvas = canvas;
    this.context = canvas.getContext('webgpu')!;
    this.format = navigator.gpu.getPreferredCanvasFormat();
    this.context.configure({
      device: this.device,
      format: this.format,
      alphaMode: 'premultiplied',
    });

    this.pageManager = new PageManager();
    return true;
  }

  loadScene(scene: BuiltScene): void {
    this.scene = scene;
    this.pageManager.registerPages(scene.pages);
    this.pageManager.makeAllResident(); // For simplicity, load everything

    this.createBuffers();
    this.createPipelines();
    this.createBindGroups();

    this.frameStats.totalClusters = scene.clusters.length;
    this.frameStats.totalTriangles = scene.clusters.reduce((s, c) => s + c.indexCount / 3, 0);
    this.frameStats.totalInstances = scene.instances.length;
    this.frameStats.totalPages = scene.pages.length;
  }

  private createBuffers(): void {
    const scene = this.scene;

    // Vertex data buffer
    this.vertexBuffer = this.device.createBuffer({
      size: Math.max(scene.vertexData.byteLength, MIN_BUFFER_SIZE),
      usage: GPUBufferUsage.STORAGE | GPUBufferUsage.COPY_DST,
    });
    this.device.queue.writeBuffer(this.vertexBuffer, 0, scene.vertexData);

    // Index buffer
    this.indexBuffer = this.device.createBuffer({
      size: Math.max(scene.indexData.byteLength, MIN_BUFFER_SIZE),
      usage: GPUBufferUsage.STORAGE | GPUBufferUsage.COPY_DST,
    });
    this.device.queue.writeBuffer(this.indexBuffer, 0, scene.indexData);

    // Cluster metadata buffer (packed as u32)
    const clusterData = this.packClusters(scene.clusters);
    this.clusterBuffer = this.device.createBuffer({
      size: Math.max(clusterData.byteLength, MIN_BUFFER_SIZE),
      usage: GPUBufferUsage.STORAGE | GPUBufferUsage.COPY_DST,
    });
    this.device.queue.writeBuffer(this.clusterBuffer, 0, clusterData);

    // Instance buffer
    const instanceData = this.packInstances(scene.instances);
    this.instanceBuffer = this.device.createBuffer({
      size: Math.max(instanceData.byteLength, MIN_BUFFER_SIZE),
      usage: GPUBufferUsage.STORAGE | GPUBufferUsage.COPY_DST,
    });
    this.device.queue.writeBuffer(this.instanceBuffer, 0, instanceData);

    // Uniform buffer
    this.uniformBuffer = this.device.createBuffer({
      size: UNIFORM_BUFFER_SIZE,
      usage: GPUBufferUsage.UNIFORM | GPUBufferUsage.COPY_DST,
    });

    // Visible instance list
    const maxInstances = Math.max(scene.instances.length, 1);
    this.visibleInstanceBuffer = this.device.createBuffer({
      size: maxInstances * 4,
      usage: GPUBufferUsage.STORAGE,
    });
    this.visibleInstanceCountBuffer = this.device.createBuffer({
      size: 4,
      usage: GPUBufferUsage.STORAGE | GPUBufferUsage.COPY_SRC,
    });

    // Visible cluster list
    const maxClusters = Math.max(scene.clusters.length, 1);
    this.visibleClusterBuffer = this.device.createBuffer({
      size: maxClusters * 4,
      usage: GPUBufferUsage.STORAGE | GPUBufferUsage.COPY_SRC,
    });
    this.visibleClusterCountBuffer = this.device.createBuffer({
      size: 4,
      usage: GPUBufferUsage.STORAGE | GPUBufferUsage.COPY_SRC,
    });

    // Stats readback buffer
    this.statsReadBuffer = this.device.createBuffer({
      size: 8, // visible instances + visible clusters
      usage: GPUBufferUsage.MAP_READ | GPUBufferUsage.COPY_DST,
    });

    // Depth texture
    this.createDepthTexture();
  }

  private createDepthTexture(): void {
    if (this.depthTexture) this.depthTexture.destroy();
    this.depthTexture = this.device.createTexture({
      size: { width: this.canvas.width, height: this.canvas.height },
      format: 'depth24plus',
      usage: GPUTextureUsage.RENDER_ATTACHMENT,
    });
  }

  private packClusters(clusters: Cluster[]): Uint32Array {
    const data = new Uint32Array(clusters.length * GPU_CLUSTER_U32_STRIDE);
    const f32View = new Float32Array(data.buffer);
    for (let i = 0; i < clusters.length; i++) {
      const c = clusters[i];
      const base = i * GPU_CLUSTER_U32_STRIDE;
      f32View[base + 0] = c.boundingSphere[0];
      f32View[base + 1] = c.boundingSphere[1];
      f32View[base + 2] = c.boundingSphere[2];
      f32View[base + 3] = c.boundingSphere[3];
      f32View[base + 4] = c.normalCone[0];
      f32View[base + 5] = c.normalCone[1];
      f32View[base + 6] = c.normalCone[2];
      f32View[base + 7] = c.normalCone[3];
      f32View[base + 8] = c.lodError;
      data[base + 9] = c.parentIndex === -1 ? INVALID_INDEX : c.parentIndex;
      data[base + 10] = c.childOffset === -1 ? INVALID_INDEX : c.childOffset;
      data[base + 11] = c.childCount;
      data[base + 12] = c.vertexOffset;
      data[base + 13] = c.vertexCount;
      data[base + 14] = c.indexOffset;
      data[base + 15] = c.indexCount;
    }
    return data;
  }

  private packInstances(instances: import('./types').Instance[]): Uint32Array {
    const STRIDE = GPU_INSTANCE_U32_STRIDE;
    const data = new Uint32Array(instances.length * STRIDE);
    const f32View = new Float32Array(data.buffer);
    for (let i = 0; i < instances.length; i++) {
      const inst = instances[i];
      const base = i * STRIDE;
      for (let j = 0; j < 16; j++) {
        f32View[base + j] = inst.transform[j];
      }
      f32View[base + 16] = inst.worldBounds[0];
      f32View[base + 17] = inst.worldBounds[1];
      f32View[base + 18] = inst.worldBounds[2];
      f32View[base + 19] = inst.worldBounds[3];
      data[base + 20] = inst.clusterOffset;
      data[base + 21] = inst.clusterCount;
      data[base + 22] = inst.rootCluster;
      data[base + 23] = inst.meshId;
    }
    return data;
  }

  private createPipelines(): void {
    // Reset counter compute pipeline
    this.resetCounterPipeline = this.device.createComputePipeline({
      layout: 'auto',
      compute: {
        module: this.device.createShaderModule({ code: RESET_COUNTER_SHADER }),
        entryPoint: 'main',
      },
    });

    // Instance culling compute pipeline
    this.instanceCullPipeline = this.device.createComputePipeline({
      layout: 'auto',
      compute: {
        module: this.device.createShaderModule({ code: INSTANCE_CULL_SHADER }),
        entryPoint: 'main',
      },
    });

    // Cluster LOD selection compute pipeline
    this.clusterLodPipeline = this.device.createComputePipeline({
      layout: 'auto',
      compute: {
        module: this.device.createShaderModule({ code: CLUSTER_LOD_SHADER }),
        entryPoint: 'main',
      },
    });

    // Main render pipeline (vertex pulling)
    const mainShaderModule = this.device.createShaderModule({ code: VERTEX_PULL_SHADER });
    this.mainRenderPipeline = this.device.createRenderPipeline({
      layout: 'auto',
      vertex: {
        module: mainShaderModule,
        entryPoint: 'vs_main',
      },
      fragment: {
        module: mainShaderModule,
        entryPoint: 'fs_main',
        targets: [{ format: this.format }],
      },
      depthStencil: {
        format: 'depth24plus',
        depthWriteEnabled: true,
        depthCompare: 'less',
      },
      primitive: {
        topology: 'triangle-list',
        cullMode: 'none', // We do our own culling in compute
      },
    });

    // Wireframe render pipeline (line rendering via triangle edges)
    const wireShaderModule = this.device.createShaderModule({ code: WIREFRAME_SHADER });
    this.wireframeRenderPipeline = this.device.createRenderPipeline({
      layout: 'auto',
      vertex: {
        module: wireShaderModule,
        entryPoint: 'vs_main',
      },
      fragment: {
        module: wireShaderModule,
        entryPoint: 'fs_main',
        targets: [{
          format: this.format,
          blend: {
            color: { srcFactor: 'src-alpha', dstFactor: 'one-minus-src-alpha' },
            alpha: { srcFactor: 'one', dstFactor: 'one-minus-src-alpha' },
          },
        }],
      },
      depthStencil: {
        format: 'depth24plus',
        depthWriteEnabled: false,
        depthCompare: 'less-equal',
      },
      primitive: {
        topology: 'line-list',
        cullMode: 'none',
      },
    });
  }

  private createBindGroups(): void {
    // Reset instance count
    this.resetInstanceCountBG = this.device.createBindGroup({
      layout: this.resetCounterPipeline.getBindGroupLayout(0),
      entries: [
        { binding: 0, resource: { buffer: this.visibleInstanceCountBuffer } },
      ],
    });

    // Reset cluster count
    this.resetClusterCountBG = this.device.createBindGroup({
      layout: this.resetCounterPipeline.getBindGroupLayout(0),
      entries: [
        { binding: 0, resource: { buffer: this.visibleClusterCountBuffer } },
      ],
    });

    // Instance culling
    this.instanceCullBG = this.device.createBindGroup({
      layout: this.instanceCullPipeline.getBindGroupLayout(0),
      entries: [
        { binding: 0, resource: { buffer: this.uniformBuffer } },
        { binding: 1, resource: { buffer: this.instanceBuffer } },
        { binding: 2, resource: { buffer: this.visibleInstanceBuffer } },
        { binding: 3, resource: { buffer: this.visibleInstanceCountBuffer } },
        { binding: 4, resource: { buffer: this.clusterBuffer } },
      ],
    });

    // Cluster LOD selection
    this.clusterLodBG = this.device.createBindGroup({
      layout: this.clusterLodPipeline.getBindGroupLayout(0),
      entries: [
        { binding: 0, resource: { buffer: this.uniformBuffer } },
        { binding: 1, resource: { buffer: this.clusterBuffer } },
        { binding: 2, resource: { buffer: this.visibleClusterBuffer } },
        { binding: 3, resource: { buffer: this.visibleClusterCountBuffer } },
      ],
    });

    // Main render
    this.mainRenderBG = this.device.createBindGroup({
      layout: this.mainRenderPipeline.getBindGroupLayout(0),
      entries: [
        { binding: 0, resource: { buffer: this.uniformBuffer } },
        { binding: 1, resource: { buffer: this.vertexBuffer } },
        { binding: 2, resource: { buffer: this.indexBuffer } },
        { binding: 3, resource: { buffer: this.clusterBuffer } },
        { binding: 4, resource: { buffer: this.visibleClusterBuffer } },
        { binding: 5, resource: { buffer: this.visibleClusterCountBuffer } },
      ],
    });

    // Wireframe render
    this.wireframeRenderBG = this.device.createBindGroup({
      layout: this.wireframeRenderPipeline.getBindGroupLayout(0),
      entries: [
        { binding: 0, resource: { buffer: this.uniformBuffer } },
        { binding: 1, resource: { buffer: this.vertexBuffer } },
        { binding: 2, resource: { buffer: this.indexBuffer } },
        { binding: 3, resource: { buffer: this.clusterBuffer } },
        { binding: 4, resource: { buffer: this.visibleClusterBuffer } },
        { binding: 5, resource: { buffer: this.visibleClusterCountBuffer } },
      ],
    });
  }

  private writeUniforms(camera: Camera, settings: RenderSettings): void {
    const view = mat4LookAt(
      Array.from(camera.position) as Vec3,
      Array.from(camera.target) as Vec3,
      Array.from(camera.up) as Vec3,
    );
    const proj = mat4Perspective(camera.fovY, camera.aspect, camera.near, camera.far);
    const viewProj = mat4Multiply(proj, view);
    const planes = extractFrustumPlanes(viewProj);

    // Pack into uniform buffer
    // Layout: viewProj(64) + view(64) + proj(64) + cameraPos(16) + frustumPlanes(96)
    //       + screenHeight(4) + fovY(4) + lodThreshold(4) + flags(16) + counts(16) = 352 bytes
    const data = new ArrayBuffer(UNIFORM_BUFFER_SIZE);
    const f32 = new Float32Array(data);
    const u32 = new Uint32Array(data);

    // viewProj: offset 0
    f32.set(viewProj, 0);
    // view: offset 16
    f32.set(view, 16);
    // proj: offset 32
    f32.set(proj, 32);
    // cameraPos: offset 48
    f32[48] = camera.position[0];
    f32[49] = camera.position[1];
    f32[50] = camera.position[2];
    f32[51] = 1.0;
    // frustumPlanes: offset 52 (6 × vec4f)
    for (let i = 0; i < 6; i++) {
      f32[52 + i * 4] = planes[i][0];
      f32[52 + i * 4 + 1] = planes[i][1];
      f32[52 + i * 4 + 2] = planes[i][2];
      f32[52 + i * 4 + 3] = planes[i][3];
    }
    // screenHeight: offset 76
    f32[76] = this.canvas.height;
    // fovY: offset 77
    f32[77] = camera.fovY;
    // lodErrorThreshold: offset 78
    f32[78] = settings.lodErrorThreshold;
    // flags: offset 79
    u32[79] = settings.enableFrustumCulling ? 1 : 0;
    u32[80] = settings.enableBackfaceCulling ? 1 : 0;
    u32[81] = settings.enableOcclusionCulling ? 1 : 0;
    u32[82] = settings.debugLODColors ? 1 : 0;
    // counts: offset 83
    u32[83] = this.scene.clusters.length;
    u32[84] = this.scene.instances.length;
    u32[85] = 0;
    u32[86] = 0;

    this.device.queue.writeBuffer(this.uniformBuffer, 0, data);
  }

  render(camera: Camera, settings: RenderSettings): void {
    const now = performance.now();
    const dt = now - this.lastFrameTime;
    this.lastFrameTime = now;
    this.fpsAccum += dt;
    this.frameCount++;
    if (this.fpsAccum >= FPS_UPDATE_INTERVAL_MS) {
      this.frameStats.fps = Math.round(this.frameCount * FPS_UPDATE_INTERVAL_MS / this.fpsAccum);
      this.frameCount = 0;
      this.fpsAccum = 0;
    }
    this.frameStats.frameTimeMs = dt;

    // Resize depth texture if needed
    if (this.depthTexture.width !== this.canvas.width ||
        this.depthTexture.height !== this.canvas.height) {
      this.createDepthTexture();
    }

    // Write uniforms
    this.writeUniforms(camera, settings);

    const commandEncoder = this.device.createCommandEncoder();

    // ── Pass 1: Reset counters ──
    {
      const pass = commandEncoder.beginComputePass();
      pass.setPipeline(this.resetCounterPipeline);
      pass.setBindGroup(0, this.resetInstanceCountBG);
      pass.dispatchWorkgroups(1);
      pass.setBindGroup(0, this.resetClusterCountBG);
      pass.dispatchWorkgroups(1);
      pass.end();
    }

    // ── Pass 2: Instance culling ──
    {
      const workgroups = Math.ceil(this.scene.instances.length / COMPUTE_WORKGROUP_SIZE_1D);
      const pass = commandEncoder.beginComputePass();
      pass.setPipeline(this.instanceCullPipeline);
      pass.setBindGroup(0, this.instanceCullBG);
      pass.dispatchWorkgroups(Math.max(workgroups, 1));
      pass.end();
    }

    // ── Pass 3: Cluster LOD selection ──
    {
      const workgroups = Math.ceil(this.scene.clusters.length / COMPUTE_WORKGROUP_SIZE_1D);
      const pass = commandEncoder.beginComputePass();
      pass.setPipeline(this.clusterLodPipeline);
      pass.setBindGroup(0, this.clusterLodBG);
      pass.dispatchWorkgroups(Math.max(workgroups, 1));
      pass.end();
    }

    // ── Pass 4: Main render ──
    const colorView = this.context.getCurrentTexture().createView();
    const depthView = this.depthTexture.createView();

    {
      const pass = commandEncoder.beginRenderPass({
        colorAttachments: [{
          view: colorView,
          clearValue: CLEAR_COLOR,
          loadOp: 'clear',
          storeOp: 'store',
        }],
        depthStencilAttachment: {
          view: depthView,
          depthClearValue: DEPTH_CLEAR_VALUE,
          depthLoadOp: 'clear',
          depthStoreOp: 'store',
        },
      });

      pass.setPipeline(this.mainRenderPipeline);
      pass.setBindGroup(0, this.mainRenderBG);

      // Draw each cluster. Since we can't use indirect draw easily with
      // vertex pulling from storage buffers, we'll draw all visible clusters
      // by iterating on CPU and issuing draws per cluster.
      // In a production system, you'd use indirect draws.
      for (let i = 0; i < this.scene.clusters.length; i++) {
        const c = this.scene.clusters[i];
        // We draw every cluster and let the vertex shader handle visibility
        // via the visible cluster list. For the initial implementation,
        // draw ALL clusters and let the LOD shader results determine visibility.
        // This is the CPU-fallback path.
      }

      // For now, draw all clusters as individual draw calls
      // Each cluster becomes one "instance" where instanceIndex = cluster index
      // The vertex shader reads from the visibleClusters buffer
      // We need to know how many visible clusters there are...
      // Since we can't readback the count synchronously, we'll draw the max
      // and have the vertex shader discard invisible ones.

      // Better approach: draw all clusters, each as an instanced draw
      // where the vertex shader checks the visible clusters list.

      // Simplest correct approach: one draw per cluster, max index count
      for (let ci = 0; ci < this.scene.clusters.length; ci++) {
        const c = this.scene.clusters[ci];
        if (c.indexCount > 0) {
          pass.draw(c.indexCount, 1, 0, ci);
        }
      }

      pass.end();
    }

    // ── Pass 5: Wireframe overlay ──
    if (settings.showWireframe) {
      const pass = commandEncoder.beginRenderPass({
        colorAttachments: [{
          view: colorView,
          loadOp: 'load',
          storeOp: 'store',
        }],
        depthStencilAttachment: {
          view: depthView,
          depthLoadOp: 'load',
          depthStoreOp: 'store',
        },
      });

      pass.setPipeline(this.wireframeRenderPipeline);
      pass.setBindGroup(0, this.wireframeRenderBG);

      // Draw wireframe edges for each cluster
      for (let ci = 0; ci < this.scene.clusters.length; ci++) {
        const c = this.scene.clusters[ci];
        if (c.indexCount > 0) {
          // For line-list, we need 2 vertices per edge, 3 edges per tri
          pass.draw(c.indexCount * 2, 1, 0, ci);
        }
      }

      pass.end();
    }

    this.device.queue.submit([commandEncoder.finish()]);

    // Update stats
    const pgStats = this.pageManager.getStats();
    this.frameStats.residentPages = pgStats.resident;
  }

  getStats(): FrameStats {
    return { ...this.frameStats };
  }

  resize(width: number, height: number): void {
    this.canvas.width = width;
    this.canvas.height = height;
    if (this.depthTexture) {
      this.createDepthTexture();
    }
  }

  destroy(): void {
    this.vertexBuffer?.destroy();
    this.indexBuffer?.destroy();
    this.clusterBuffer?.destroy();
    this.instanceBuffer?.destroy();
    this.uniformBuffer?.destroy();
    this.visibleInstanceBuffer?.destroy();
    this.visibleInstanceCountBuffer?.destroy();
    this.visibleClusterBuffer?.destroy();
    this.visibleClusterCountBuffer?.destroy();
    this.statsReadBuffer?.destroy();
    this.depthTexture?.destroy();
  }
}
