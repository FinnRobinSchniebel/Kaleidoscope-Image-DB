import * as THREE from "three";
import { TUNNEL_TILE_PATCH } from "@/components/KscopeSharedUI/KaleidoscopeTunnel/tunnelTilePatch.ts";
import type { TunnelTile } from "@/components/KscopeSharedUI/KaleidoscopeTunnel/tunnelTilePatch.types.ts";
import { loadTunnelTileGeometry } from "./tunnelTileAsset.ts";
import { PATCH_U_MIN, PATCH_U_MAX, PATCH_UV_SCALE, computeTileFrame, type ConeConfig, type Vec2 } from "./tunnelConeMath.ts";

// One InstancedMesh per mirror state, each using its own pre-mirrored (or
// not) geometry -- see tunnelTileAsset.ts's buildMirroredGeometry for why
// this replaced negating a basis vector per-instance at render time.
// `paletteIndex` is the tile's index in the *original* (unsplit)
// TUNNEL_TILE_PATCH, so `palette[paletteIndex % palette.length]` still
// cycles exactly like the SVG version's own `i % palette.length`, unaffected
// by which group a tile landed in after splitting.
type TileGroup = {
  readonly mesh: THREE.InstancedMesh;
  readonly entries: readonly { readonly tile: TunnelTile; readonly paletteIndex: number }[];
};

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

// Mirrors KaleidoscopeTunnelBackgroundProps -- everything the SVG version's
// own useMemo folds into a ConeConfig, minus `phase` (driven by the render
// loop, passed separately to update() since it changes every tick while
// these change rarely, if ever).
export type TunnelSceneProps = {
  readonly turns: number;
  readonly baseWidth: number;
  readonly startDepth: number;
  readonly depth: number;
  readonly slantWeight: number;
  readonly tipFocusX: number;
  readonly tipFocusY: number;
  readonly baseFocusX: number;
  readonly baseFocusY: number;
};

// ambientIntensity is the brightness floor: tuned so the darkest-lit tile
// face never reads as harder to see than the old flat color did, with
// key/fill only ever adding contrast on top of it, never subtracting
// below it. See LightingDebugPanel.tsx for why there's no light/dark-theme
// split.
export type TunnelLighting = {
  readonly ambientIntensity: number;
  readonly keyIntensity: number;
  readonly fillIntensity: number;
};

// note: keyIntensity is ~1000x ambient/fill on purpose, not a typo. Three.js
// point-light falloff is intensity / distance^decay, and this scene's world
// units are real CSS-px-equivalents (tile-to-tip distances run into the
// thousands), so an intensity sized like an ordinary small-scene light
// (single digits) is numerically indistinguishable from zero at that range.
export const DEFAULT_LIGHTING: TunnelLighting = {
  ambientIntensity: 0.6,
  keyIntensity: 15000,
  fillIntensity: 1.5,
};

// Glossy-glass look: zero metalness, since metalness would tint the
// specular highlight with the tile's own palette color instead of keeping
// it neutral. MeshPhysicalMaterial (not Standard) is required for
// clearcoat, which is what makes roughness visibly affect a low-metalness
// dielectric -- its own base specular (F0 ~= 0.04) is too faint against
// the dominant diffuse IBL term for roughness to show through otherwise.
// See CONTEXT.md section 9 for the full investigation.
export type TunnelMaterial = {
  readonly roughness: number;
  readonly metalness: number;
  readonly envMapIntensity: number;
  readonly clearcoat: number;
};

// envMapIntensity's default (0.25) is well below three.js's own default of
// 1 -- at 1, the environment's diffuse contribution alone overexposes
// tiles to washed-out white even with every point light at 0 (see
// CONTEXT.md section 9).
export const DEFAULT_MATERIAL: TunnelMaterial = {
  roughness: 0.2,
  metalness: 0,
  envMapIntensity: 0.25,
  clearcoat: 0,
};

// A low-roughness dielectric alone barely reflects anything visible (F0 ~=
// 0.04) without an environment to reflect. Builds an approximate
// equirectangular environment (same palette as the container's own CSS
// gradient, see KaleidoscopeTunnelBackgroundGL.tsx) rather than a literal
// port of it -- a 2D screen-space gradient has no exact translation into a
// direction-based reflection map. Real per-tile or per-tile-to-tile
// reflection is deferred; see CONTEXT.md section 9.
//
// note: the highlight blobs give roughness real high-frequency detail to
// blur. A perfectly smooth gradient has none, which is why roughness 0 and
// 1 used to look nearly identical without them (see CONTEXT.md section 9).
function buildGradientEnvironment(renderer: THREE.WebGLRenderer): THREE.Texture {
  const width = 128;
  const height = 64;
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d")!;

  const base = ctx.createLinearGradient(0, 0, 0, height);
  base.addColorStop(0, "#ffffff");
  base.addColorStop(0.3, "#99f6e4");
  base.addColorStop(1, "#77c2ff");
  ctx.fillStyle = base;
  ctx.fillRect(0, 0, width, height);

  // Kept low (peak alpha 0.22, small radii) so the blobs add texture
  // without dominating the average brightness.
  const blobs: readonly { x: number; y: number; r: number }[] = [
    { x: width * 0.2, y: height * 0.15, r: width * 0.06 },
    { x: width * 0.75, y: height * 0.3, r: width * 0.045 },
    { x: width * 0.5, y: height * 0.78, r: width * 0.07 },
  ];
  for (const blob of blobs) {
    const glow = ctx.createRadialGradient(blob.x, blob.y, 0, blob.x, blob.y, blob.r);
    glow.addColorStop(0, "rgba(255,255,255,0.22)");
    glow.addColorStop(1, "rgba(255,255,255,0)");
    ctx.fillStyle = glow;
    ctx.fillRect(blob.x - blob.r, blob.y - blob.r, blob.r * 2, blob.r * 2);
  }

  const texture = new THREE.CanvasTexture(canvas);
  texture.mapping = THREE.EquirectangularReflectionMapping;
  texture.colorSpace = THREE.SRGBColorSpace;

  const pmrem = new THREE.PMREMGenerator(renderer);
  const envMap = pmrem.fromEquirectangular(texture).texture;
  pmrem.dispose();
  texture.dispose();
  return envMap;
}

// Owns the actual three.js scene/camera/InstancedMesh for the tunnel tiles,
// independent of React -- KaleidoscopeTunnelBackgroundGL.tsx just drives its
// lifecycle (load/update/render/dispose) from effects. update() recomputes
// every instance's transform from scratch each call rather than diffing
// props, which is negligible cost at this tile count and lets props/phase
// change freely without a separate dirty-tracking scheme.
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
  // Ambient: the brightness floor (uniform, no position/direction). Key: a
  // point light near the cone's tip -- positioned in updateLighting() each
  // call since the tip's world position (0, 0, zFar) moves with the
  // depth/startDepth props. Fill: parented to the camera, so its world
  // position tracks the camera for free via the normal scene-graph
  // transform. Fill is given a nonzero local offset (see constructor)
  // rather than sitting at the camera's own origin -- a light exactly
  // coincident with the camera only produces a visible specular highlight
  // where a surface faces the camera dead-on (N ~= view direction), and
  // every tile here sits at an oblique angle from the cone/spiral wrap, so
  // that condition is never met.
  private readonly ambientLight = new THREE.AmbientLight(0xffffff, 0);
  private readonly keyLight = new THREE.PointLight(0xffffff, 0, 0, 1);
  private readonly fillLight = new THREE.PointLight(0xffffff, 0, 0, 0);
  private material: THREE.MeshPhysicalMaterial | null = null;
  private groups: readonly TileGroup[] = [];
  private environment: THREE.Texture | null = null;

  constructor() {
    this.worldGroup.scale.y = -1;
    this.scene.add(this.worldGroup);
    this.scene.add(this.ambientLight);
    this.scene.add(this.keyLight);
    // Local to the camera: -X is left, +Z is behind (camera looks down its
    // own -Z). See fillLight's own comment above for why this can't just
    // sit at the camera's origin.
    this.fillLight.position.set(-500, -500, 0);
    this.camera.add(this.fillLight);
    this.scene.add(this.camera);
  }

  async load(palette: readonly string[], materialSettings: TunnelMaterial = DEFAULT_MATERIAL): Promise<void> {
    const geometry = await loadTunnelTileGeometry();
    // Instance tinting (setColorAt below) works independently of this
    // material -- it's gated on InstancedMesh.instanceColor existing, not
    // on a vertexColors flag (there's no per-vertex geometry color, see
    // tunnelTileAsset.ts). alphaMap is the per-tile "fades toward
    // transparent at center, opaque at the edge" rim mask, baked into a
    // static texture (tunnelTileAsset.ts's buildRimAlphaTexture) rather
    // than a per-vertex attribute, since this mesh's cap has no interior
    // vertex to fade toward -- every vertex sits exactly on the boundary.
    const material = new THREE.MeshPhysicalMaterial({
      side: THREE.DoubleSide,
      alphaMap: geometry.rimAlphaMap,
      transparent: true,
      roughness: materialSettings.roughness,
      metalness: materialSettings.metalness,
      envMapIntensity: materialSettings.envMapIntensity,
      clearcoat: materialSettings.clearcoat,
      clearcoatRoughness: materialSettings.roughness,
    });
    const patch = DEBUG_U_RANGE
      ? TUNNEL_TILE_PATCH.filter((t) => t.u > DEBUG_U_RANGE[0] && t.u < DEBUG_U_RANGE[1])
      : TUNNEL_TILE_PATCH;

    const buildGroup = (geo: THREE.BufferGeometry, mirrored: boolean): TileGroup | null => {
      const entries = patch
        .map((tile, paletteIndex) => ({ tile, paletteIndex }))
        .filter((e) => e.tile.mirrored === mirrored);
      if (entries.length === 0) return null;
      const mesh = new THREE.InstancedMesh(geo, material, entries.length);
      this.worldGroup.add(mesh);
      return { mesh, entries };
    };

    this.groups = [buildGroup(geometry.normal, false), buildGroup(geometry.mirrored, true)].filter(
      (g): g is TileGroup => g !== null,
    );
    this.material = material;
    this.setPalette(palette);
  }

  // Cheap material-property update -- called from its own effect keyed on
  // material settings, same pattern as setPalette (rare changes, not
  // per-frame).
  setMaterialSettings(materialSettings: TunnelMaterial): void {
    if (!this.material) return;
    this.material.roughness = materialSettings.roughness;
    this.material.metalness = materialSettings.metalness;
    this.material.envMapIntensity = materialSettings.envMapIntensity;
    this.material.clearcoat = materialSettings.clearcoat;
    this.material.clearcoatRoughness = materialSettings.roughness;
  }

  // Cheap enough to just always reapply (patch.length calls to setColorAt)
  // -- palette changes are rare (a prop, not phase), so this is called from
  // its own effect keyed on the palette prop, not from the per-frame loop.
  setPalette(palette: readonly string[]): void {
    const color = new THREE.Color();
    for (const group of this.groups) {
      group.entries.forEach(({ paletteIndex }, i) => {
        group.mesh.setColorAt(i, color.set(palette[paletteIndex % palette.length]));
      });
      if (group.mesh.instanceColor) group.mesh.instanceColor.needsUpdate = true;
    }
  }

  // rNear/baseOffset (and so every tile's position) are genuine px
  // quantities, not just an aspect ratio, so this needs the real container
  // size, not merely width/height's ratio.
  update(props: TunnelSceneProps, lighting: TunnelLighting, phase: number, width: number, height: number): void {
    if (this.groups.length === 0 || width <= 0 || height <= 0) return;
    const cfg = this.buildConfig(props, phase, width, height);
    this.updateCamera(width, height, cfg, props.tipFocusX, props.tipFocusY);
    this.updateLighting(lighting, cfg);
    this.layout(cfg);
  }

  // Ambient is the brightness floor (see DEFAULT_LIGHTING's comment); key
  // sits at the cone's tip -- world (0, 0, zFar) exactly, since frameAt's
  // baseOffset taper is always 0 there regardless of baseFocus/tipFocus, so
  // no worldGroup Y-flip correction is needed (its own y is always 0) --
  // and its `decay` (1, set once at construction) gives real but gentler-
  // than-physical distance falloff along the cone, per the migration plan's
  // "distance falloff, not a hard facing cutoff". Fill has no decay at all
  // (a flat camera-relative boost, not meant to fall off with scene depth).
  private updateLighting(lighting: TunnelLighting, cfg: ConeConfig): void {
    this.ambientLight.intensity = lighting.ambientIntensity;
    this.keyLight.intensity = lighting.keyIntensity;
    this.keyLight.position.set(0, 0, cfg.zFar);
    this.fillLight.intensity = lighting.fillIntensity;
  }

  // Mirrors KaleidoscopeTunnelBackground.tsx's own useMemo -- baseOffset is
  // the mouth-to-tip world-space axis tilt that makes the mouth's screen
  // projection land on baseFocus while the tip lands on tipFocus (tipFocus
  // itself is applied separately below, as a camera-level shift, since
  // frameAt's taper already zeroes baseOffset out at the tip). Same clamps
  // as the SVG version too (Math.max(0.01, baseWidth) etc.), so out-of-range
  // props can't break the render there either.
  private buildConfig(props: TunnelSceneProps, phase: number, width: number, height: number): ConeConfig {
    const startDepth = Math.min(props.startDepth, PERSPECTIVE - 100);
    const pfNear = PERSPECTIVE / (PERSPECTIVE - startDepth);
    const baseOffset: Vec2 = [
      (width * (props.baseFocusX - props.tipFocusX)) / pfNear,
      (height * (props.baseFocusY - props.tipFocusY)) / pfNear,
    ];
    return {
      uMin: PATCH_U_MIN,
      uMax: PATCH_U_MAX,
      uvScale: PATCH_UV_SCALE,
      turns: props.turns,
      slantWeight: Math.min(1, Math.max(0, props.slantWeight)),
      rNear: (Math.max(0.01, props.baseWidth) * Math.max(width, height)) / 2,
      zNear: startDepth,
      zFar: startDepth - Math.max(1, props.depth),
      baseOffset,
      phase,
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
  private updateCamera(width: number, height: number, cfg: ConeConfig, tipFocusX: number, tipFocusY: number): void {
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

    const shiftX = (tipFocusX - 0.5) * width;
    const shiftY = (tipFocusY - 0.5) * height;
    this.camera.setViewOffset(width, height, -shiftX, -shiftY, width, height);
    this.camera.updateProjectionMatrix();
  }

  // Converts each patch tile's cone-wrapped position/basis (tunnelConeMath)
  // into a per-instance matrix. `right`/`down`/`normal` are used exactly as
  // computed -- never negated here -- since mirroring is handled entirely
  // by which geometry a tile's group uses (see tunnelTileAsset.ts's
  // buildMirroredGeometry). Every instance therefore gets a real rotation
  // (never a reflection), which is what keeps WebGL's single per-draw-call
  // front-face convention correct for all of them; see that same comment
  // for why negating a basis vector per-instance broke lighting specifically
  // for mirrored tiles.
  private layout(cfg: ConeConfig): void {
    const matrix = new THREE.Matrix4();
    const position = new THREE.Vector3();
    const right = new THREE.Vector3();
    const down = new THREE.Vector3();
    const normal = new THREE.Vector3();

    for (const group of this.groups) {
      group.entries.forEach(({ tile }, i) => {
        const frame = computeTileFrame(tile, cfg);
        position.set(...frame.position);
        right.set(...frame.right);
        down.set(...frame.down);
        normal.crossVectors(right, down).normalize();

        matrix.makeBasis(right, down, normal).setPosition(position);
        group.mesh.setMatrixAt(i, matrix);
      });
      group.mesh.instanceMatrix.needsUpdate = true;
    }
  }

  render(renderer: THREE.WebGLRenderer): void {
    // Built lazily here (needs a WebGLRenderer to prefilter with, which
    // TunnelScene otherwise never holds onto -- it's only ever passed in,
    // per-call, from the outside) rather than in load()/the constructor.
    if (!this.environment) {
      this.environment = buildGradientEnvironment(renderer);
      this.scene.environment = this.environment;
      // note: also assigned directly to the material, not just left as the
      // scene-level default -- this is load-bearing, not redundant. Per
      // WebGLRenderer.js, any material relying on scene.environment as an
      // implicit fallback (material.envMap left null) gets its own
      // envMapIntensity uniform silently overwritten by
      // scene.environmentIntensity (default 1) instead, making
      // material.envMapIntensity inert.
      if (this.material) this.material.envMap = this.environment;
    }
    renderer.render(this.scene, this.camera);
  }

  dispose(): void {
    this.material?.dispose();
    this.environment?.dispose();
    for (const group of this.groups) group.mesh.dispose();
  }
}
