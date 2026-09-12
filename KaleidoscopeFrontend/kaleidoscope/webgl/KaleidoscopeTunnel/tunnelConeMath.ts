// Describes TUNNEL_TILE_PATCH's own baked data (see tunnelTilePatch.ts's
// header) -- not an independent tuning knob. Ported from PATCH_SHAPE in the
// SVG version's KaleidoscopeTunnelBackground.tsx; changing these without
// regenerating TUNNEL_TILE_PATCH makes every tile's position/size wrong.
export const PATCH_U_MIN = -120;
export const PATCH_U_MAX = 124;
export const PATCH_UV_SCALE = 38.709678;

// The cone/spiral wrap itself (frameAt in tunnelSpiral.ts) lands here in M3.
