import type { TunnelTile } from "./tunnelTilePatch.types.ts";

type Vec3 = readonly [number, number, number];

function add(a: Vec3, b: Vec3): Vec3 {
  return [a[0] + b[0], a[1] + b[1], a[2] + b[2]];
}
function sub(a: Vec3, b: Vec3): Vec3 {
  return [a[0] - b[0], a[1] - b[1], a[2] - b[2]];
}
function mul(a: Vec3, s: number): Vec3 {
  return [a[0] * s, a[1] * s, a[2] * s];
}
function dot(a: Vec3, b: Vec3): number {
  return a[0] * b[0] + a[1] * b[1] + a[2] * b[2];
}
function norm(a: Vec3): Vec3 {
  const len = Math.hypot(a[0], a[1], a[2]) || 1;
  return mul(a, 1 / len);
}

type Vec2 = readonly [number, number];

// V0, the hat monotile's true (unshrunk) local origin, in the same local
// box as TILE_LOCAL_POINTS below; matches hatviz's rotate/mirror-about-V0
// placement convention.
const PIVOT_X = 89.032;
const PIVOT_Y = 52.258;

// Hat monotile clip shape (3.5px perpendicular inset per edge), as local
// (x, y) points. Source of truth for projectTile's local-to-screen mapping.
const TILE_LOCAL_POINTS: readonly Vec2[] = [
  [85.53, 54.28], [85.53, 22.23], [67.66, 22.23], [58.72, 6.75],
  [34.47, 20.76], [34.47, 55.76], [13.63, 55.76], [6.71, 67.75],
  [30.97, 81.76], [61.28, 64.25], [71.70, 82.31], [106.37, 82.31],
  [113.28, 70.31],
];

export type ConeConfig = {
  readonly uMin: number;
  readonly uMax: number;
  readonly uvScale: number;
  readonly turns: number;
  readonly zNear: number;
  readonly zFar: number;
  readonly rNear: number;
  // 0 = seed Gram-Schmidt from the radial direction, 1 = from the cone's
  // own slant direction. See frameAt's comment for the tradeoff this
  // blends between.
  readonly slantWeight: number;
  // World-space (x, y) the mouth's axis point is shifted by, tapering
  // linearly to 0 at the tip -- tilts the cone's axis so the mouth and tip
  // can aim at different screen points instead of sharing one.
  readonly baseOffset: Vec2;
  // Radians added to every tile's theta -- spins the spiral around its own
  // (possibly tilted) axis. Can't be done as a post-hoc 2D transform once
  // baseOffset is nonzero, since the axis no longer passes through a
  // single shared screen point at every depth.
  readonly phase: number;
};

// Local frame (position + tangent/across) at flat-strip parameter x. Gorth
// (the across-strip direction) must be exactly perpendicular to That (the
// tangent), or mixing them via each tile's own rotation gives inconsistent
// tile sizes - so it's Gram-Schmidt-orthogonalized from a seed blended by
// slantWeight. Slant-seeded keeps tiles edge-connected to their neighbors
// but tilts them edge-on (thin slivers, since slant leans into depth for
// this tunnel's proportions); radial-seeded keeps tiles face-on but leaves
// gaps near the wide mouth.
function frameAt(x: number, cfg: ConeConfig) {
  const xMax = (cfg.uMax - cfg.uMin) * cfg.uvScale;
  const omega = (2 * Math.PI * cfg.turns) / xMax;
  const k = (cfg.zFar - cfg.zNear) / xMax;
  const rPrime = -cfg.rNear / xMax;

  const theta = omega * x + cfg.phase;
  const r = cfg.rNear * (1 - x / xMax);
  const z = cfg.zNear + k * x;

  // Tapers baseOffset from full strength at the mouth (x = 0) to 0 at the
  // tip (x = xMax), so the axis is a straight 3D line between the two.
  const taper = 1 - x / xMax;
  const taperPrime = -1 / xMax;
  const ox = cfg.baseOffset[0] * taper;
  const oy = cfg.baseOffset[1] * taper;

  const C: Vec3 = [r * Math.cos(theta) + ox, r * Math.sin(theta) + oy, z];

  const T: Vec3 = [
    rPrime * Math.cos(theta) - r * omega * Math.sin(theta) + cfg.baseOffset[0] * taperPrime,
    rPrime * Math.sin(theta) + r * omega * Math.cos(theta) + cfg.baseOffset[1] * taperPrime,
    k,
  ];
  // |T| is the wrap's local stretch factor relative to the flat (pre-wrap)
  // patch: x is already in physical px there, so |T| = 1 is no stretch,
  // |T| = 2 covers twice the ground the flat layout assumed. It grows
  // toward the wide mouth (larger r * omega term). `scale` (see
  // projectTile) is exactly this value, used unnormalized - do not rescale
  // it against any single reference point (e.g. the tip's own value),
  // since that reference shifts with turns/depth/size and would silently
  // weaken the compensation.
  const speed = Math.hypot(T[0], T[1], T[2]);
  const That = mul(T, 1 / (speed || 1));

  const m = cfg.rNear / (cfg.zNear - cfg.zFar);
  const radial: Vec3 = [Math.cos(theta), Math.sin(theta), 0];
  const slant: Vec3 = [-m * Math.cos(theta), -m * Math.sin(theta), 1];
  const w = cfg.slantWeight;
  const seed: Vec3 = norm([
    (1 - w) * radial[0] + w * slant[0],
    (1 - w) * radial[1] + w * slant[1],
    (1 - w) * radial[2] + w * slant[2],
  ]);
  const Gorth = norm(sub(seed, mul(That, dot(seed, That))));

  return { C, That, Gorth, scale: speed, r };
}

export type ProjectedTile = {
  readonly points: string; // ready-made SVG <polygon points> value
  readonly z: number; // world depth, for painter's-algorithm sort order
};

// Projects each of the 13 polygon vertices individually, perspective-
// divided by its own depth, rather than one shared factor evaluated at the
// pivot: a single factor is only accurate when a tile's depth variation
// across its face is small relative to camera distance, which breaks for
// the largest (most `scale`-inflated) tiles near the mouth. Computed here
// in JS rather than via a per-tile matrix3d + preserve-3d, which forces
// the browser to promote every tile to its own compositing layer.
export function projectTile(
  tile: TunnelTile,
  cfg: ConeConfig,
  perspective: number,
): ProjectedTile {
  const x = (tile.u - cfg.uMin) * cfg.uvScale;
  const vOffset = tile.v * cfg.uvScale;
  const { C, That, Gorth, scale, r } = frameAt(x, cfg);
  // Shrinks v-offset toward 0 as r approaches 0: a fixed-width v-band
  // can't fit around a shrinking circumference without this. Gorth is a
  // constant unit vector (unlike That), so it needs no `scale` compensation.
  const vTaper = r / (Math.hypot(r, vOffset) || 1);
  const P = add(C, mul(Gorth, vOffset * vTaper));

  const rot = (tile.rot * Math.PI) / 180;
  const c = Math.cos(rot);
  const sn = Math.sin(rot);

  // Scaled in the (That, Gorth) basis before rotation is mixed in, not on
  // right/down directly, since which local axis ends up That-aligned
  // depends on each tile's own rot.
  const scaledThat = mul(That, scale);
  // rotate(rot) scaleX(mirror) composes as p' = R(rot)*S(mirror)*p, so
  // mirroring flips only the local x-hat image ("right"); "down" is
  // unaffected.
  let right = add(mul(scaledThat, c), mul(Gorth, sn));
  if (tile.mirrored) right = mul(right, -1);
  const down = add(mul(scaledThat, -sn), mul(Gorth, c));

  const points = TILE_LOCAL_POINTS.map(([lx, ly]) => {
    const ox = lx - PIVOT_X;
    const oy = ly - PIVOT_Y;
    const wx = P[0] + right[0] * ox + down[0] * oy;
    const wy = P[1] + right[1] * ox + down[1] * oy;
    const wz = P[2] + right[2] * ox + down[2] * oy;
    const pf = perspective / (perspective - wz);
    return `${wx * pf},${wy * pf}`;
  }).join(" ");

  return { points, z: P[2] };
}

export function projectTunnelTiles(
  patch: readonly TunnelTile[],
  cfg: ConeConfig,
  perspective: number,
): ProjectedTile[] {
  return patch.map((t) => projectTile(t, cfg, perspective));
}
