import {
  TURTLE_FIELD_PLACEMENTS,
  TURTLE_FIELD_VIEWBOX,
} from "./turtleFieldPlacements.ts";

// The artwork's own extent, in the same "1 unit = one short tile edge" system
// the placements use -- i.e. turtle-monotile-kites-fresnel.svg's viewBox. These
// two stay in sync by construction rather than by discipline: the generator
// derives its placements from the very kite paths that file draws.
const TILE_W = 6;
const TILE_H = 4.33012702;
const TILE_CENTER_X = 3;
const TILE_CENTER_Y = 2.165063512;
const GAP_SCALE = 0.98;

export type TurtleFieldBackgroundProps = {
  /**
   * Whole-layer multiplier on top of the per-kite alphas baked into the
   * artwork. This sits behind a spiral already drawn at fillOpacity 0.5-0.7,
   * so it is meant to read as texture rather than compete with it; lower this
   * if it starts to.
   */
  opacity?: number;
};

/**
 * A static field of turtle monotiles, drawn as their ten-kite decomposition in
 * semi-transparent white so the container's radial gradient shows through a
 * faceted aperiodic texture. Sits between that gradient and the spiral.
 *
 * Unlike the spiral, this measures nothing and animates nothing: the viewBox is
 * fixed, so there is no size state, no ResizeObserver and no mount gate, and it
 * renders identically on the server and the client.
 *
 * The placement table is already exactly the tiles that can appear, so there is
 * no filtering here either -- see turtleFieldPlacements.ts's header.
 */
// Applied right to left: shrink toward TILE_CENTER_X/Y first, then mirror,
// rotate, and translate -- placement instead pivots on the artwork's origin (V0).
function tileTransform(t: { x: number; y: number; rot: number; mirrored: boolean }): string {
  return (
    `translate(${t.x},${t.y}) rotate(${t.rot})` +
    (t.mirrored ? " scale(-1,1)" : "") +
    ` translate(${TILE_CENTER_X},${TILE_CENTER_Y}) scale(${GAP_SCALE}) translate(${-TILE_CENTER_X},${-TILE_CENTER_Y})`
  );
}

function TurtleTiles() {
  return (
    <>
      {TURTLE_FIELD_PLACEMENTS.map((t, i) => (
        <image
          key={i}
          href="/turtle-monotile-kites-fresnel.svg"
          width={TILE_W}
          height={TILE_H}
          transform={tileTransform(t)}
        />
      ))}
    </>
  );
}

export default function TurtleFieldBackground({
  opacity = 1,
}: TurtleFieldBackgroundProps = {}) {
  const { x, y, w, h } = TURTLE_FIELD_VIEWBOX;

  return (
    <svg
      className="absolute inset-0 h-full w-full"
      viewBox={`${x} ${y} ${w} ${h}`}
      preserveAspectRatio="xMidYMid slice"
      opacity={opacity}
      aria-hidden="true"
      // Isolates this static layer so the spiral's ~20fps repaint doesn't
      // force it (mask included) to re-rasterize every frame instead of once.
      style={{ willChange: "transform" }}
    >
      <defs>
        {/* Forced solid black, not just opaque: a luminance mask reads color,
            so an opaque white pixel wouldn't cut a hole. The *20 covers
            alpha as low as 0.06 (the artwork's faintest kites). */}
        <filter id="turtleSolid">
          <feColorMatrix
            type="matrix"
            values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 20 0"
          />
        </filter>
        <mask id="turtleGaps" maskUnits="userSpaceOnUse" x={x} y={y} width={w} height={h}>
          <rect x={x} y={y} width={w} height={h} fill="white" />
          <g filter="url(#turtleSolid)">
            <TurtleTiles />
          </g>
        </mask>
      </defs>

      <rect x={x} y={y} width={w} height={h} fill="white" fillOpacity={0.7} mask="url(#turtleGaps)" />

      <TurtleTiles />
    </svg>
  );
}
