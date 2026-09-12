import * as THREE from "three";
import { TUNNEL_TILE_PATCH } from "@/components/KscopeSharedUI/KaleidoscopeTunnel/tunnelTilePatch.ts";
import { loadTunnelTileGeometry } from "./tunnelTileAsset.ts";
import { PATCH_U_MIN, PATCH_U_MAX, PATCH_UV_SCALE } from "./tunnelConeMath.ts";

// DEBUG ONLY -- set to [uMin, uMax] to render just that spatial slice
// (TUNNEL_TILE_PATCH isn't stored in spatial order, so array slicing
// doesn't give neighbors) for close inspection; null renders everything.
// Must be null before calling M2 done.
const DEBUG_U_RANGE: [number, number] | null = null;

// Owns the actual three.js scene/camera/InstancedMesh for the tunnel tiles,
// independent of React -- KaleidoscopeTunnelBackgroundGL.tsx just drives its
// lifecycle (load/setAspect/render/dispose) from effects.
export class TunnelScene {
  readonly scene = new THREE.Scene();
  readonly camera = new THREE.PerspectiveCamera(50, 1, 1, 1);
  private mesh: THREE.InstancedMesh | null = null;
  private material: THREE.MeshBasicMaterial | null = null;
  private bounds: THREE.Box3 | null = null;
  private lastAspect: number | null = null;

  // M2: places every patch tile at its flat (u, v) position -- no cone wrap
  // yet (that's frameAt, landing in M3). Mirroring negates `right` the same
  // way projectTile does, which flips handedness; DoubleSide sidesteps the
  // resulting wrong-way normal on mirrored tiles for now. That'll need a
  // real fix (flip the normal back, not just render both sides) once M5's
  // lighting actually depends on normal direction.
  async load(palette: readonly string[]): Promise<void> {
    const geometry = await loadTunnelTileGeometry();
    geometry.computeBoundingSphere();
    const tileRadius = geometry.boundingSphere?.radius ?? 0;
    const material = new THREE.MeshBasicMaterial({ side: THREE.DoubleSide, vertexColors: true });
    const patch = DEBUG_U_RANGE
      ? TUNNEL_TILE_PATCH.filter((t) => t.u > DEBUG_U_RANGE[0] && t.u < DEBUG_U_RANGE[1])
      : TUNNEL_TILE_PATCH;
    const mesh = new THREE.InstancedMesh(geometry, material, patch.length);

    const uMid = (PATCH_U_MIN + PATCH_U_MAX) / 2;
    const matrix = new THREE.Matrix4();
    const position = new THREE.Vector3();
    const right = new THREE.Vector3();
    const down = new THREE.Vector3();
    const normal = new THREE.Vector3();
    const color = new THREE.Color();
    const bounds = new THREE.Box3();

    patch.forEach((tile, i) => {
      position.set((tile.u - uMid) * PATCH_UV_SCALE, tile.v * PATCH_UV_SCALE, 0);

      const rot = (tile.rot * Math.PI) / 180;
      const c = Math.cos(rot);
      const s = Math.sin(rot);
      right.set(c, s, 0);
      down.set(-s, c, 0);
      // Computed from the unmirrored `right` -- mirroring must only flip
      // the in-plane footprint (right), not the extrusion direction. Doing
      // this after negating right would flip normal's sign too (right and
      // normal both negated = a proper rotation, not a reflection), which
      // extrudes a mirrored tile's depth toward the camera instead of away
      // from it like every other tile.
      normal.crossVectors(right, down);
      if (tile.mirrored) right.multiplyScalar(-1);

      matrix.makeBasis(right, down, normal).setPosition(position);
      mesh.setMatrixAt(i, matrix);
      mesh.setColorAt(i, color.set(palette[i % palette.length]));
      bounds.expandByPoint(position);
    });

    mesh.instanceMatrix.needsUpdate = true;
    if (mesh.instanceColor) mesh.instanceColor.needsUpdate = true;

    // `bounds` so far only covers tile *centers* -- pad by each tile's own
    // extent, or a small/nearby cluster (like the debug view) can end up
    // with a smaller box than the tiles actually drawn, putting the camera
    // closer than the geometry itself.
    bounds.expandByScalar(tileRadius);
    this.bounds = bounds;
    this.scene.add(mesh);
    this.mesh = mesh;
    this.material = material;
    // A placeholder aspect here is fine -- setAspect always re-fits on its
    // very first real call (lastAspect starts null), before render() is
    // ever reached, so this never produces a visibly-wrong frame.
    this.frameCamera(1.5);
  }

  // Fits the camera tight to whatever was actually placed, for the current
  // aspect ratio -- TUNNEL_TILE_PATCH is a long, thin strip (~9445 x ~232
  // units). Fitting a bounding sphere via vertical FOV alone (the previous
  // approach) is aspect-independent by construction, which left most of a
  // typical wide viewport blank: it sizes to whichever axis the sphere is
  // tightest on regardless of how much wider the viewport actually is.
  // Picks whichever of width/height is the binding constraint instead.
  private frameCamera(aspect: number): void {
    if (!this.bounds) return;
    const size = this.bounds.getSize(new THREE.Vector3());
    const center = this.bounds.getCenter(new THREE.Vector3());
    const margin = 1.1;
    const halfFovV = THREE.MathUtils.degToRad(this.camera.fov / 2);
    const distanceForHeight = (size.y / 2) * margin / Math.tan(halfFovV);
    const distanceForWidth = (size.x / 2) * margin / (Math.tan(halfFovV) * aspect);
    const distance = Math.max(distanceForHeight, distanceForWidth, 1);
    const depthMargin = Math.max(size.x, size.y, 10);
    this.camera.position.set(center.x, center.y, center.z + distance);
    this.camera.near = Math.max(1, distance - depthMargin);
    this.camera.far = distance + depthMargin;
    this.camera.aspect = aspect;
    this.camera.lookAt(center);
    this.camera.updateProjectionMatrix();
  }

  setAspect(aspect: number): void {
    if (this.lastAspect === aspect) return;
    this.lastAspect = aspect;
    if (this.bounds) {
      this.frameCamera(aspect);
    } else {
      this.camera.aspect = aspect;
      this.camera.updateProjectionMatrix();
    }
  }

  render(renderer: THREE.WebGLRenderer): void {
    renderer.render(this.scene, this.camera);
  }

  dispose(): void {
    this.material?.dispose();
    this.mesh?.dispose();
  }
}
