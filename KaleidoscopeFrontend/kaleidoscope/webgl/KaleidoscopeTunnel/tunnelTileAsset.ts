import * as THREE from "three";
import { STLLoader } from "three/addons/loaders/STLLoader.js";
import { TILE_LOCAL_STROKE_WIDTH } from "@/components/KscopeSharedUI/KaleidoscopeTunnel/tunnelSpiral.ts";

// Fit against hat-monotile.stl's true (unshrunk) outline, not
// tunnelSpiral.ts's TILE_LOCAL_POINTS directly (a 3.5px inset of the true
// shape) -- see CONTEXT.md section 4 for the derivation. Don't hand-tune
// without redoing that fit.
const XY_SCALE = 3.87152;
const PIVOT_X_RAW = 10;
const PIVOT_Y_RAW = 0;
// By-eye constant: the STL's modeled depth, scaled by XY_SCALE alone,
// reads flatter than intended since most tiles sit close to face-on to the
// camera. Tune against the live scene; see CONTEXT.md section 4.
const DEPTH_DAMPING = 2;

// The 13 boundary points of hat-monotile.stl's front/back cap outline, in
// the STL's own raw coordinates (extracted once by walking the front-cap
// triangles' boundary edges -- the edges that belong to exactly one
// front-cap triangle rather than two). Winds the same direction as
// TILE_LOCAL_POINTS (turning-angle sum -360), which is what fixes the sign
// in insetPolygon below.
const STL_BOUNDARY_RAW: readonly (readonly [number, number])[] = [
  [-5, 0], [-5, 8.660125732421875], [2.5, 12.990264892578125], [5, 8.660125732421875],
  [10, 8.660125732421875], [10, 0], [17.5, -4.329986572265625], [15, -8.660125732421875],
  [5, -8.660125732421875], [2.5, -4.329986572265625], [-5, -8.660125732421875],
  [-12.5, -4.329986572265625], [-10, 0],
];
// Perpendicular inset distance (STL's own raw units) to open a small gap
// between neighbors. note: must be a true perpendicular inset, not a
// uniform scale toward the pivot -- the pivot isn't centered on the shape,
// so scaling toward it retreats each tile's copy of a shared edge by a
// different amount and direction depending on that tile's own rotation,
// producing a gap that overlaps at one end and opens at the other instead
// of a uniform width. See CONTEXT.md section 4.
const GAP_INSET = 0.5;

// Offsets a closed polygon by `dist` along each edge's outward normal,
// re-intersecting consecutive offset edges for the new vertices -- the same
// technique used during calibration to reconstruct the true (un-inset) hat
// shape, run here with a negative distance to inset instead of outset.
function offsetPolygon(points: readonly (readonly [number, number])[], dist: number): [number, number][] {
  const n = points.length;
  const lines: { p: [number, number]; d: [number, number] }[] = [];
  for (let i = 0; i < n; i++) {
    const a = points[i];
    const b = points[(i + 1) % n];
    const d: [number, number] = [b[0] - a[0], b[1] - a[1]];
    const len = Math.hypot(d[0], d[1]) || 1;
    const nx = (-d[1] / len) * dist;
    const ny = (d[0] / len) * dist;
    lines.push({ p: [a[0] + nx, a[1] + ny], d });
  }
  const result: [number, number][] = [];
  for (let i = 0; i < n; i++) {
    const l1 = lines[(i - 1 + n) % n];
    const l2 = lines[i];
    const [x1, y1] = l1.p;
    const [dx1, dy1] = l1.d;
    const [x2, y2] = l2.p;
    const [dx2, dy2] = l2.d;
    const denom = dx1 * dy2 - dy1 * dx2;
    const t = ((x2 - x1) * dy2 - (y2 - y1) * dx2) / denom;
    result.push([x1 + t * dx1, y1 + t * dy1]);
  }
  return result;
}

function keyOf(x: number, y: number): string {
  return `${x.toFixed(2)},${y.toFixed(2)}`;
}

// The mesh's actual boundary polygon (13 points) in the same final local
// coordinate units as the geometry's own vertices -- i.e. STL_BOUNDARY_RAW
// carried through the exact same inset/pivot/scale pipeline as the real
// geometry below, so this traces precisely what the mesh's own silhouette
// is, not an approximation of it. Used both to know where to put the rim
// texture's UVs and what shape to actually draw into it.
function computeFinalBoundary(): readonly (readonly [number, number])[] {
  return offsetPolygon(STL_BOUNDARY_RAW, -GAP_INSET).map(
    ([x, y]): [number, number] => [(x - PIVOT_X_RAW) * XY_SCALE, (y - PIVOT_Y_RAW) * -XY_SCALE],
  );
}

// Every vertex's (x, y) is one of `boundary`'s 13 points (see
// insetTileVertices's comment -- caps and walls alike), so mapping those 13
// points into a shared [0,1] UV square and matching every vertex against
// them (same keyOf trick as insetTileVertices) gives every vertex a UV
// that's exactly where its own (x, y) sits on the tile's real footprint --
// not an approximation via a bounding circle or box interior.
function addRimUVs(geometry: THREE.BufferGeometry, boundary: readonly (readonly [number, number])[]): void {
  const xs = boundary.map((p) => p[0]);
  const ys = boundary.map((p) => p[1]);
  const minX = Math.min(...xs), maxX = Math.max(...xs);
  const minY = Math.min(...ys), maxY = Math.max(...ys);
  const remap = new Map<string, [number, number]>();
  boundary.forEach(([x, y]) => remap.set(keyOf(x, y), [(x - minX) / (maxX - minX), (y - minY) / (maxY - minY)]));

  const posAttr = geometry.getAttribute("position") as THREE.BufferAttribute;
  const uv = new Float32Array(posAttr.count * 2);
  for (let i = 0; i < posAttr.count; i++) {
    const found = remap.get(keyOf(posAttr.getX(i), posAttr.getY(i))) ?? [0.5, 0.5];
    uv[i * 2] = found[0];
    uv[i * 2 + 1] = found[1];
  }
  geometry.setAttribute("uv", new THREE.BufferAttribute(uv, 2));
}

// Multiplier on TILE_LOCAL_STROKE_WIDTH -- same rim-light knob as the SVG
// version's own RIM_WIDTH_SCALE (KaleidoscopeTunnelBackground.tsx), not
// imported from there since that file's default export isn't meant to be a
// shared module -- kept as a separate, independently-tunable constant here.
const RIM_WIDTH_SCALE = 0.4;

// Shortest distance from (px, py) to the segment [a, b].
function distanceToSegment(
  px: number,
  py: number,
  ax: number,
  ay: number,
  bx: number,
  by: number,
): number {
  const dx = bx - ax, dy = by - ay;
  const lenSq = dx * dx + dy * dy || 1;
  const t = Math.max(0, Math.min(1, ((px - ax) * dx + (py - ay) * dy) / lenSq));
  return Math.hypot(px - (ax + t * dx), py - (ay + t * dy));
}

// Bakes the SVG version's per-tile "fades toward transparent at center,
// opaque at the edge" mask (KaleidoscopeTunnelBackground.tsx's
// tileRimMask) into a static alphaMap texture instead: that mask was
// rebuilt as an SVG <mask> from every tile's live projected points on every
// phase tick, which is exactly the per-frame CPU cost this migration
// exists to eliminate. The tile's local shape never changes frame to
// frame, only its instance transform does, so the *pattern* can be baked
// once and just resampled per-instance via UVs (addRimUVs above) instead.
// alphaMap reads a texture's green channel (see three.js's own
// alphamap_fragment.glsl.js), not a real alpha channel, hence the flat
// gray fill rather than a transparent PNG.
//
// note: every pixel's value is a function of its distance to the nearest
// boundary *edge* (min over all 13 segments), not a stroked/blurred path --
// a stroke's line-join at each vertex (miter by default) can overshoot on
// this shape's sharp concave corners, producing bright spikes there and a
// comparatively thin line along long straight edges between them. A pure
// distance field has no join to overshoot: the rim width only ever depends
// on distance from the true edge, uniform all the way around regardless of
// how densely the boundary's vertices happen to be spaced.
function buildRimAlphaTexture(boundary: readonly (readonly [number, number])[]): THREE.CanvasTexture {
  const SIZE = 256;
  const xs = boundary.map((p) => p[0]);
  const ys = boundary.map((p) => p[1]);
  const minX = Math.min(...xs), maxX = Math.max(...xs);
  const minY = Math.min(...ys), maxY = Math.max(...ys);
  const toCanvas = ([x, y]: readonly [number, number]): [number, number] => [
    ((x - minX) / (maxX - minX)) * SIZE,
    ((y - minY) / (maxY - minY)) * SIZE,
  ];
  const canvasBoundary = boundary.map(toCanvas);
  const n = canvasBoundary.length;

  const canvas = document.createElement("canvas");
  canvas.width = SIZE;
  canvas.height = SIZE;
  const ctx = canvas.getContext("2d")!;

  // Opaque only within halfWidth of the true edge, then a long linear
  // falloff over featherPx back down to the base interior value (rgb 64,
  // matching the SVG mask's fillOpacity={0.25}). halfWidth is kept small
  // relative to featherPx deliberately: a sharp convex spike (this shape
  // has several) is locally thin, so its whole tip sits within a fixed
  // distance of *some* edge -- too large a halfWidth reads as a solid,
  // ungraded patch of full opacity right at those tips instead of a rim.
  const strokeWidthPx = ((TILE_LOCAL_STROKE_WIDTH * RIM_WIDTH_SCALE) / (maxX - minX)) * SIZE;
  const halfWidth = strokeWidthPx * 0.15;
  const featherPx = strokeWidthPx * 2;

  const image = ctx.createImageData(SIZE, SIZE);
  const data = image.data;
  for (let py = 0; py < SIZE; py++) {
    for (let px = 0; px < SIZE; px++) {
      let minDist = Infinity;
      for (let i = 0; i < n; i++) {
        const [ax, ay] = canvasBoundary[i];
        const [bx, by] = canvasBoundary[(i + 1) % n];
        const d = distanceToSegment(px + 0.5, py + 0.5, ax, ay, bx, by);
        if (d < minDist) minDist = d;
      }
      const t = Math.max(0, Math.min(1, (minDist - halfWidth) / featherPx));
      const value = 255 + (64 - 255) * t;
      const idx = (py * SIZE + px) * 4;
      data[idx] = data[idx + 1] = data[idx + 2] = value;
      data[idx + 3] = 255;
    }
  }
  ctx.putImageData(image, 0, 0);

  const texture = new THREE.CanvasTexture(canvas);
  texture.colorSpace = THREE.NoColorSpace;
  texture.needsUpdate = true;
  return texture;
}

// Every vertex in the mesh (front cap, back cap, and side walls alike) has
// an (x, y) matching one of the 13 boundary points, at one of two depths --
// so remapping just those 13 (x, y) values shrinks the whole extruded
// prism's silhouette uniformly at every depth, not only its front face.
function insetTileVertices(geometry: THREE.BufferGeometry, dist: number): void {
  const insetPoints = offsetPolygon(STL_BOUNDARY_RAW, -dist);
  const remap = new Map<string, [number, number]>();
  STL_BOUNDARY_RAW.forEach((p, i) => remap.set(keyOf(p[0], p[1]), insetPoints[i]));

  const posAttr = geometry.getAttribute("position") as THREE.BufferAttribute;
  for (let i = 0; i < posAttr.count; i++) {
    const replacement = remap.get(keyOf(posAttr.getX(i), posAttr.getY(i)));
    if (replacement) posAttr.setXY(i, replacement[0], replacement[1]);
  }
  posAttr.needsUpdate = true;
}

// Builds the mirrored-tile geometry variant: negates local X on every
// vertex and swaps each triangle's last two vertices to restore correct
// winding (an X-flip alone reverses it, which would make mirrored caps
// face backwards). Requires `base` to already carry a "uv" attribute
// (addRimUVs); mirrors it (u' = 1-u) too, so the shared rim texture
// samples correctly for these tiles.
//
// note: this exists so TunnelScene can give mirrored tiles a real
// rotation-only instance matrix instead of negating a basis vector at
// render time. The two look equivalent (same vertex positions either way)
// but are not for lighting: WebGL determines front-facing winding once per
// draw call from the mesh's own world matrix, not per instance, so a
// per-instance reflection reads as lit from behind. See CONTEXT.md section
// 3 for the full derivation -- don't go back to negating a basis vector
// per instance to "simplify" this.
function buildMirroredGeometry(base: THREE.BufferGeometry): THREE.BufferGeometry {
  const mirrored = base.clone();
  const pos = mirrored.getAttribute("position") as THREE.BufferAttribute;
  const uv = mirrored.getAttribute("uv") as THREE.BufferAttribute;
  for (let t = 0; t < pos.count / 3; t++) {
    const i0 = t * 3;
    const i1 = t * 3 + 1;
    const i2 = t * 3 + 2;
    pos.setX(i0, -pos.getX(i0));
    pos.setX(i1, -pos.getX(i1));
    pos.setX(i2, -pos.getX(i2));
    uv.setX(i0, 1 - uv.getX(i0));
    uv.setX(i1, 1 - uv.getX(i1));
    uv.setX(i2, 1 - uv.getX(i2));
    const x1 = pos.getX(i1), y1 = pos.getY(i1), z1 = pos.getZ(i1);
    pos.setXYZ(i1, pos.getX(i2), pos.getY(i2), pos.getZ(i2));
    pos.setXYZ(i2, x1, y1, z1);
    const u1 = uv.getX(i1), v1 = uv.getY(i1);
    uv.setXY(i1, uv.getX(i2), uv.getY(i2));
    uv.setXY(i2, u1, v1);
  }
  pos.needsUpdate = true;
  uv.needsUpdate = true;
  mirrored.computeVertexNormals();
  return mirrored;
}

export type TunnelTileGeometry = {
  readonly normal: THREE.BufferGeometry;
  readonly mirrored: THREE.BufferGeometry;
  readonly rimAlphaMap: THREE.Texture;
};

let cached: Promise<TunnelTileGeometry> | null = null;

// Loads and prepares the tunnel tile geometry exactly once; every caller
// shares the same promise/geometries/texture rather than re-fetching or
// re-building them.
export function loadTunnelTileGeometry(): Promise<TunnelTileGeometry> {
  if (!cached) {
    cached = new STLLoader().loadAsync("/hat-monotile.stl").then((geometry) => {
      // Inset before the pivot-translate/flip/scale below, in the STL's own
      // raw axes where STL_BOUNDARY_RAW was measured.
      insetTileVertices(geometry, GAP_INSET);
      // Translate first (in the STL's own raw axes, before the Y-flip
      // below), then flip+scale -- see the derivation above for why Y (not
      // X) carries the sign flip here.
      geometry.translate(-PIVOT_X_RAW, -PIVOT_Y_RAW, 0);
      geometry.scale(XY_SCALE, -XY_SCALE, XY_SCALE * DEPTH_DAMPING);
      geometry.computeVertexNormals();

      const boundary = computeFinalBoundary();
      addRimUVs(geometry, boundary);

      return {
        normal: geometry,
        mirrored: buildMirroredGeometry(geometry),
        rimAlphaMap: buildRimAlphaTexture(boundary),
      };
    });
  }
  return cached;
}
