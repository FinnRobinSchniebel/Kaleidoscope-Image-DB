import * as THREE from "three";
import { TUNNEL_TILE_PATCH } from "@/components/KscopeSharedUI/KaleidoscopeTunnel/tunnelTilePatch.ts";
import type { TunnelTile } from "@/components/KscopeSharedUI/KaleidoscopeTunnel/tunnelTilePatch.types.ts";
import { loadTunnelTileGeometry } from "./tunnelTileAsset.ts";
import { PATCH_U_MIN, PATCH_U_MAX, PATCH_UV_SCALE, computeTileFrame, type ConeConfig, type Vec2 } from "./tunnelConeMath.ts";

// DEBUG ONLY -- set to [uMin, uMax] to render just that spatial slice
// (TUNNEL_TILE_PATCH isn't stored in spatial order, so array slicing
// doesn't give neighbors) for close inspection; null renders everything.
const DEBUG_U_RANGE: [number, number] | null = null;

// Same reference distance as the SVG version's PERSPECTIVE constant (see
// KaleidoscopeTunnelBackground.tsx) -- the camera sits this many world units
// in front of the z=0 plane, along +Z. Not a prop: matching this exactly
// (not just "some perspective-looking value") is what makes frameAt/
// computeTileFrame's ported math reproduce the SVG version's shape, since
// world units there are implicitly "CSS px at z=0".
const PERSPECTIVE = 1800;

// M3: fixed reference config matching the real values app/(app)/layout.tsx
// passes to <TunnelBackground> today (not KaleidoscopeTunnelBackground.tsx's
// own bare defaults, which neither real mount site actually uses), so the
// two implementations' output can be compared directly at an actual
// production config. Not wired to props/animated phase yet -- that's M4.
const TURNS = 2.5;
const BASE_WIDTH = 1.5;
const START_DEPTH = 0;
const DEPTH = 9000;
const SLANT_WEIGHT = 0.0;
const TIP_FOCUS: Vec2 = [0.95, 0.2];
const BASE_FOCUS: Vec2 = [-1, 1.9];
const PHASE = 0;

// Owns the actual three.js scene/camera/InstancedMesh for the tunnel tiles,
// independent of React -- KaleidoscopeTunnelBackgroundGL.tsx just drives its
// lifecycle (load/setSize/render/dispose) from effects.
export class TunnelScene {
  readonly scene = new THREE.Scene();
  readonly camera = new THREE.PerspectiveCamera();
  // tunnelSpiral.ts's math (ported in tunnelConeMath.ts) treats +y as
  // "down the screen", matching SVG/CSS pixel convention -- three.js's
  // camera instead treats +y as up. Rather than thread a sign flip through
  // every ported formula, the whole tile group is mirrored across the X
  // axis once here, so position/right/down/normal math above this line can
  // stay a direct, literal port with no convention translation of its own.
  private readonly worldGroup = new THREE.Group();
  private mesh: THREE.InstancedMesh | null = null;
  private material: THREE.MeshBasicMaterial | null = null;
  private patch: readonly TunnelTile[] = [];
  private lastWidth = -1;
  private lastHeight = -1;

  constructor() {
    this.worldGroup.scale.y = -1;
    this.scene.add(this.worldGroup);
  }

  async load(palette: readonly string[]): Promise<void> {
    const geometry = await loadTunnelTileGeometry();
    const material = new THREE.MeshBasicMaterial({ side: THREE.DoubleSide, vertexColors: true });
    const patch = DEBUG_U_RANGE
      ? TUNNEL_TILE_PATCH.filter((t) => t.u > DEBUG_U_RANGE[0] && t.u < DEBUG_U_RANGE[1])
      : TUNNEL_TILE_PATCH;
    const mesh = new THREE.InstancedMesh(geometry, material, patch.length);

    const color = new THREE.Color();
    patch.forEach((_, i) => mesh.setColorAt(i, color.set(palette[i % palette.length])));
    if (mesh.instanceColor) mesh.instanceColor.needsUpdate = true;

    this.worldGroup.add(mesh);
    this.mesh = mesh;
    this.material = material;
    this.patch = patch;

    // A size may already have been reported before load() resolved; lay
    // out immediately with it instead of waiting for the next resize.
    if (this.lastWidth > 0 && this.lastHeight > 0) {
      this.layout(this.buildConfig(this.lastWidth, this.lastHeight));
    }
  }

  // rNear/baseOffset (and so every tile's position) are genuine px
  // quantities, not just an aspect ratio, so -- unlike M2's auto-fit camera
  // -- this needs the real container size, not merely width/height's ratio.
  setSize(width: number, height: number): void {
    if (width === this.lastWidth && height === this.lastHeight) return;
    this.lastWidth = width;
    this.lastHeight = height;
    if (width <= 0 || height <= 0) return;

    const cfg = this.buildConfig(width, height);
    this.updateCamera(width, height, cfg);
    if (this.mesh) this.layout(cfg);
  }

  // Mirrors KaleidoscopeTunnelBackground.tsx's own useMemo -- baseOffset is
  // the mouth-to-tip world-space axis tilt that makes the mouth's screen
  // projection land on baseFocus while the tip lands on tipFocus (tipFocus
  // itself is applied separately below, as a camera-level shift, since
  // frameAt's taper already zeroes baseOffset out at the tip).
  private buildConfig(width: number, height: number): ConeConfig {
    const startDepth = Math.min(START_DEPTH, PERSPECTIVE - 100);
    const pfNear = PERSPECTIVE / (PERSPECTIVE - startDepth);
    const baseOffset: Vec2 = [
      (width * (BASE_FOCUS[0] - TIP_FOCUS[0])) / pfNear,
      (height * (BASE_FOCUS[1] - TIP_FOCUS[1])) / pfNear,
    ];
    return {
      uMin: PATCH_U_MIN,
      uMax: PATCH_U_MAX,
      uvScale: PATCH_UV_SCALE,
      turns: TURNS,
      slantWeight: SLANT_WEIGHT,
      rNear: (BASE_WIDTH * Math.max(width, height)) / 2,
      zNear: startDepth,
      zFar: startDepth - DEPTH,
      baseOffset,
      phase: PHASE,
    };
  }

  // Reproduces the SVG version's `pf = PERSPECTIVE / (PERSPECTIVE - z)`
  // projection exactly: a real camera sitting PERSPECTIVE world units in
  // front of the z=0 plane, with vertical FOV chosen so that plane's world
  // units map 1:1 to CSS px, is the same single-center-of-projection
  // transform in disguise. tipFocus (the vanishing point's screen position)
  // is then an off-axis frustum shift, not a camera rotation -- rotating
  // the camera to "look at" tipFocus would introduce keystone distortion
  // the SVG version's plain 2D translate never had; shifting the frustum
  // window instead reproduces a constant screen-pixel offset at every
  // depth, matching a plain translate exactly (unlike the SVG version, this
  // can't be a post-render translate of the canvas element -- WebGL culls
  // geometry outside the camera's frustum, so content revealed at the
  // shifted edge must actually be rendered there, not just uncovered).
  private updateCamera(width: number, height: number, cfg: ConeConfig): void {
    const halfHeight = height / 2;
    this.camera.fov = THREE.MathUtils.radToDeg(2 * Math.atan(halfHeight / PERSPECTIVE));
    this.camera.aspect = width / height;
    this.camera.position.set(0, 0, PERSPECTIVE);
    this.camera.up.set(0, 1, 0);
    this.camera.lookAt(0, 0, 0);

    const distNear = PERSPECTIVE - cfg.zNear;
    const distFar = PERSPECTIVE - cfg.zFar;
    const margin = Math.max(500, Math.abs(cfg.zFar - cfg.zNear) * 0.05);
    this.camera.near = Math.max(1, Math.min(distNear, distFar) - margin);
    this.camera.far = Math.max(distNear, distFar) + margin;

    const shiftX = (TIP_FOCUS[0] - 0.5) * width;
    const shiftY = (TIP_FOCUS[1] - 0.5) * height;
    this.camera.setViewOffset(width, height, -shiftX, -shiftY, width, height);
    this.camera.updateProjectionMatrix();
  }

  // Converts each patch tile's cone-wrapped position/basis (tunnelConeMath)
  // into a per-instance matrix. Mirroring negates `right` (the local
  // x-axis's image) same as projectTile does -- but only after `normal` is
  // derived from the *unmirrored* right, since negating two of a basis's
  // three vectors is a proper rotation, not a reflection, and would flip a
  // mirrored tile's extrusion to face the camera instead of away from it.
  private layout(cfg: ConeConfig): void {
    const mesh = this.mesh;
    if (!mesh) return;

    const matrix = new THREE.Matrix4();
    const position = new THREE.Vector3();
    const right = new THREE.Vector3();
    const down = new THREE.Vector3();
    const normal = new THREE.Vector3();

    this.patch.forEach((tile, i) => {
      const frame = computeTileFrame(tile, cfg);
      position.set(...frame.position);
      right.set(...frame.right);
      down.set(...frame.down);
      normal.crossVectors(right, down).normalize();
      if (tile.mirrored) right.multiplyScalar(-1);

      matrix.makeBasis(right, down, normal).setPosition(position);
      mesh.setMatrixAt(i, matrix);
    });

    mesh.instanceMatrix.needsUpdate = true;
  }

  render(renderer: THREE.WebGLRenderer): void {
    renderer.render(this.scene, this.camera);
  }

  dispose(): void {
    this.material?.dispose();
    this.mesh?.dispose();
  }
}
