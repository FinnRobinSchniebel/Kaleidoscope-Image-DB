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
function cross(a: Vec3, b: Vec3): Vec3 {
  return [
    a[1] * b[2] - a[2] * b[1],
    a[2] * b[0] - a[0] * b[2],
    a[0] * b[1] - a[1] * b[0],
  ];
}
function norm(a: Vec3): Vec3 {
  const len = Math.hypot(a[0], a[1], a[2]) || 1;
  return mul(a, 1 / len);
}

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
};

// Local frame (position + tangent/across/normal) at flat-strip parameter x.
// That is the spiral's true tangent (a literal position derivative). Gorth
// -- the "across the strip" direction each tile's v-offset is added along
// -- must come out EXACTLY perpendicular to That: mixing two non-orthogonal
// unit vectors via each tile's own (cos rot, sin rot) rotation produces
// wildly inconsistent, sometimes near-degenerate combined vectors from
// tile to tile. So Gorth is Gram-Schmidt-orthogonalized against That from a
// starting guess, which guarantees exact perpendicularity (and unit
// length) regardless of the guess -- what the guess actually controls is
// *which* of the two directions perpendicular to That gets picked, and
// that trades off two real, opposite failure modes (both confirmed by
// rendering each extreme):
//   - seeding from the cone's own SLANT direction keeps every tile
//     properly adjacent to its neighbors (confirmed: a fully continuous,
//     gap-free spiral line) because the flat patch's rot values only stay
//     meaningful if the local frame rotates the same way its neighbors'
//     does -- but the slant direction leans heavily toward the tunnel's
//     own depth axis for this tunnel's proportions (radius barely tapers
//     over a huge depth range), so every tile also renders edge-on, as a
//     thin sliver.
//   - seeding from the RADIAL direction keeps tiles face-on (their full
//     hat shape is visible, not a sliver) but drifts away from the
//     slant-seeded frame enough to open visible gaps between neighbors,
//     worse where tiles are bigger (near the wide mouth) since the same
//     relative drift is a bigger absolute pixel gap there.
// slantWeight blends the two seeds so this can be tuned between "fully
// connected but sliver-thin" and "well-shaped but gappy" rather than
// having to fully commit to one failure mode.
function frameAt(x: number, cfg: ConeConfig) {
  const xMax = (cfg.uMax - cfg.uMin) * cfg.uvScale;
  const omega = (2 * Math.PI * cfg.turns) / xMax;
  const k = (cfg.zFar - cfg.zNear) / xMax;
  const rPrime = -cfg.rNear / xMax;

  const theta = omega * x;
  const r = cfg.rNear * (1 - x / xMax);
  const z = cfg.zNear + k * x;

  const C: Vec3 = [r * Math.cos(theta), r * Math.sin(theta), z];

  const T: Vec3 = [
    rPrime * Math.cos(theta) - r * omega * Math.sin(theta),
    rPrime * Math.sin(theta) + r * omega * Math.cos(theta),
    k,
  ];
  // |T| (the tangent's raw, un-normalized magnitude) is exactly how many
  // physical px one unit of flat-strip x actually covers once wrapped onto
  // the cone. The patch data spaces tiles at a roughly constant rate in x,
  // with no awareness of the cone at all, so this is not constant along
  // the strip: near the wide mouth, one unit of x sweeps a big arc (large
  // r * omega tangential term) -- covering far more physical distance than
  // the same unit of x does near the tip (r -> 0, tangential term
  // vanishes). Tiles rendered at a fixed size can't keep up with that, so
  // they end up spaced apart with visible gaps toward the mouth -- worse
  // the more turns the spiral makes, since each additional turn packs the
  // same tile count across proportionally less x per loop. See `scale` in
  // matrixFor, which grows tile size by this same ratio to compensate.
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

  let N = norm(cross(That, Gorth));
  const radialOut = norm([C[0], C[1], 0]);
  if (dot(N, radialOut) > 0) N = mul(N, -1);

  // Reference speed: the tangent's magnitude at the tip (r = 0), where the
  // tangential term drops out entirely -- tiles there are taken as the
  // "natural", unstretched size, and scale grows from 1 at the tip toward
  // however much bigger the mouth's tiles need to be to keep up.
  const tipSpeed = Math.hypot(rPrime, k) || 1;
  const scale = speed / tipSpeed;

  return { C, That, Gorth, N, scale };
}

export function matrixFor(tile: TunnelTile, cfg: ConeConfig): string {
  const x = (tile.u - cfg.uMin) * cfg.uvScale;
  const vOffset = tile.v * cfg.uvScale;
  const { C, That, Gorth, N, scale } = frameAt(x, cfg);
  // vOffset gets the same `scale` as tile size (see frameAt) -- Gorth is a
  // pure unit direction with no stretch of its own baked in, so without
  // this, tiles at nonzero v would grow bigger toward the mouth without
  // the gap between v-neighbors growing to match, piling them on top of
  // each other instead of just closing the along-strip gaps.
  const P = add(C, mul(Gorth, vOffset * scale));

  const rot = (tile.rot * Math.PI) / 180;
  const c = Math.cos(rot);
  const sn = Math.sin(rot);

  // rotate(rot) scaleX(mirror) composes as p' = R(rot)*S(mirror)*p, so
  // mirroring flips the image of local x-hat (this "right" column) only --
  // the image of local y-hat ("down") is untouched by scaleX. Both also
  // get the position-dependent `scale` factor so tile size grows in step
  // with the growing gap between neighbors toward the mouth.
  let right = mul(add(mul(That, c), mul(Gorth, sn)), scale);
  if (tile.mirrored) right = mul(right, -1);
  const down = mul(add(mul(That, -sn), mul(Gorth, c)), scale);

  const m = [
    right[0], right[1], right[2], 0,
    down[0], down[1], down[2], 0,
    N[0], N[1], N[2], 0,
    P[0], P[1], P[2], 1,
  ];
  return `matrix3d(${m.map((n) => n.toFixed(4)).join(",")})`;
}

export function computeTunnelTransforms(
  patch: readonly TunnelTile[],
  cfg: ConeConfig,
): string[] {
  return patch.map((t) => matrixFor(t, cfg));
}
