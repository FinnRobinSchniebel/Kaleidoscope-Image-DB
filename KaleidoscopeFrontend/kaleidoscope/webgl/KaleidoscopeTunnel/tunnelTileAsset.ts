import * as THREE from "three";
import { STLLoader } from "three/addons/loaders/STLLoader.js";

// hat-monotile.stl's front-face (z=0) outline matches the *true* (unshrunk)
// hat shape, not TILE_LOCAL_POINTS in tunnelSpiral.ts directly -- that's a
// 3.5px perpendicular inset of the true hat, a deliberate SVG-only styling
// choice (a small decorative gap between tiles), not a uniform scale of it.
// Fitting the STL against TILE_LOCAL_POINTS as-is therefore has an
// irreducible ~0.35% mismatch (insetting isn't shape-preserving), which
// showed up as visibly inconsistent gaps once tiles were instanced.
// Reconstructing the true hat outline by reversing that inset (offsetting
// each edge outward by 3.5 and re-intersecting) and fitting the STL against
// *that* instead drops the residual to ~5e-8 -- floating-point noise, i.e.
// an exact match. That fit is also what these constants come from.
const XY_SCALE = 3.87152;
// PIVOT_X/PIVOT_Y in tunnelSpiral.ts (V0) is exactly the true hat's own
// first vertex once reconstructed above -- not a coincidence, hence the
// clean round numbers here once inverted back into the STL's raw space.
const PIVOT_X_RAW = 10;
const PIVOT_Y_RAW = 0;
// The STL's modeled depth (4 units) reads much thicker than the old fake
// extrusion's few-px sliver once scaled by XY_SCALE alone -- dampened
// further here. Tune by eye against the current SVG version.
const DEPTH_DAMPING = 0.4;

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
// between neighbors. A uniform scale toward V0 was tried first and is
// wrong: V0 isn't centered on the shape, so scaling each tile toward its
// own V0 retreats every edge by an amount *and direction* that depends on
// that tile's own rotation. Two neighboring tiles' copies of what was one
// shared edge then retreat in different, non-parallel directions -- so the
// gap between them isn't just uneven width, it can overlap at one end of
// the edge while gapping at the other. A true perpendicular inset, applied
// here in local space before each tile's own rotation, keeps both tiles'
// copies of a shared edge parallel after rotation (rotation preserves
// perpendicularity), so the gap stays a uniform width regardless of either
// tile's own rotation.
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

// DEBUG ONLY -- darkens side-wall triangles (mixed z=0/z=depth vertices)
// relative to the front/back caps (uniform z), so each tile's extrusion
// direction is directly visible: this is what let us see a mirrored tile's
// depth poking toward the camera instead of away from it, confirming the
// normal-direction bug above rather than just inferring it. Needs
// `vertexColors: true` on the material to take effect.
function addSideShading(geometry: THREE.BufferGeometry, darkness: number): void {
  const posAttr = geometry.getAttribute("position") as THREE.BufferAttribute;
  const colors = new Float32Array(posAttr.count * 3);
  for (let t = 0; t < posAttr.count / 3; t++) {
    const z0 = posAttr.getZ(t * 3);
    const z1 = posAttr.getZ(t * 3 + 1);
    const z2 = posAttr.getZ(t * 3 + 2);
    const isCap = Math.abs(z0 - z1) < 1e-6 && Math.abs(z1 - z2) < 1e-6;
    const shade = isCap ? 1 : darkness;
    for (let k = 0; k < 3; k++) {
      const idx = (t * 3 + k) * 3;
      colors[idx] = colors[idx + 1] = colors[idx + 2] = shade;
    }
  }
  geometry.setAttribute("color", new THREE.BufferAttribute(colors, 3));
}

let cached: Promise<THREE.BufferGeometry> | null = null;

// Loads and prepares the tunnel tile geometry exactly once; every caller
// shares the same promise/geometry rather than re-fetching or re-scaling.
export function loadTunnelTileGeometry(): Promise<THREE.BufferGeometry> {
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
      addSideShading(geometry, 0.55);
      geometry.computeVertexNormals();
      return geometry;
    });
  }
  return cached;
}
