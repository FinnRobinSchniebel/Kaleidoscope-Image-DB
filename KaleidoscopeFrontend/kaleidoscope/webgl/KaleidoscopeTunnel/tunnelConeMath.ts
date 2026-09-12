// Pure-math port of frameAt/projectTile's position+orientation derivation
// from tunnelSpiral.ts (the SVG implementation) -- everything up through the
// pivot's world position and local (right/down) basis, but not that file's
// per-vertex SVG-polygon-string step: the mesh geometry (tunnelTileAsset.ts)
// already supplies real vertices, translated so its own local origin sits at
// tunnelSpiral.ts's PIVOT_X/PIVOT_Y, so only a per-tile position + basis is
// needed here, not a per-vertex projection. No three.js import, so this
// stays testable/verifiable in isolation from the renderer.
export type Vec3 = readonly [number, number, number];
export type Vec2 = readonly [number, number];

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

// Describes TUNNEL_TILE_PATCH's own baked data (see tunnelTilePatch.ts's
// header) -- not an independent tuning knob. Ported from PATCH_SHAPE in the
// SVG version's KaleidoscopeTunnelBackground.tsx; changing these without
// regenerating TUNNEL_TILE_PATCH makes every tile's position/size wrong.
export const PATCH_U_MIN = -120;
export const PATCH_U_MAX = 124;
export const PATCH_UV_SCALE = 38.709678;

export type ConeConfig = {
  readonly uMin: number;
  readonly uMax: number;
  readonly uvScale: number;
  readonly turns: number;
  readonly zNear: number;
  readonly zFar: number;
  readonly rNear: number;
  readonly slantWeight: number;
  readonly baseOffset: Vec2;
  readonly phase: number;
};

// Ported verbatim (formulas + comments) from tunnelSpiral.ts's frameAt --
// see that file for the derivation of slantWeight's tradeoff and why Gorth
// is Gram-Schmidt-orthogonalized rather than a fixed perpendicular.
function frameAt(x: number, cfg: ConeConfig) {
  const xMax = (cfg.uMax - cfg.uMin) * cfg.uvScale;
  const omega = (2 * Math.PI * cfg.turns) / xMax;
  const k = (cfg.zFar - cfg.zNear) / xMax;
  const rPrime = -cfg.rNear / xMax;

  const theta = omega * x + cfg.phase;
  const r = cfg.rNear * (1 - x / xMax);
  const z = cfg.zNear + k * x;

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

export type TileFrame = {
  readonly position: Vec3;
  // Unmirrored -- callers apply mirroring themselves (negate right) *after*
  // deriving anything that must stay consistent regardless of mirroring,
  // like a face normal via cross(right, down). See TunnelScene's comment on
  // why computing that from an already-mirrored right flips its sign too.
  readonly right: Vec3;
  readonly down: Vec3;
};

// Ported from projectTile's position/right/down derivation (tunnelSpiral.ts)
// minus the final per-vertex SVG projection. right/down are *not* unit
// length or orthogonal in general -- they blend That (stretched by `scale`)
// with Gorth (unit) according to the tile's own rot, same as the SVG
// version, so the instanced mesh gets the same anisotropic stretch/shear
// that keeps it glued to its neighbors across the cone wrap.
export function computeTileFrame(
  tile: { readonly u: number; readonly v: number; readonly rot: number },
  cfg: ConeConfig,
): TileFrame {
  const x = (tile.u - cfg.uMin) * cfg.uvScale;
  const vOffset = tile.v * cfg.uvScale;
  const { C, That, Gorth, scale, r } = frameAt(x, cfg);
  // Shrinks v-offset toward 0 as r approaches 0 -- see projectTile's comment.
  const vTaper = r / (Math.hypot(r, vOffset) || 1);
  const position = add(C, mul(Gorth, vOffset * vTaper));

  const rot = (tile.rot * Math.PI) / 180;
  const c = Math.cos(rot);
  const sn = Math.sin(rot);
  const scaledThat = mul(That, scale);
  const right = add(mul(scaledThat, c), mul(Gorth, sn));
  const down = add(mul(scaledThat, -sn), mul(Gorth, c));

  return { position, right, down };
}
