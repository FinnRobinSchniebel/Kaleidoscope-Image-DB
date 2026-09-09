import {
  TURTLE_FIELD_PLACEMENTS,
  TURTLE_FIELD_VIEWBOX,
} from "./turtleFieldPlacements.ts";

// The artwork's own extent, in the same "1 unit = one short tile edge" system
// the placements use -- i.e. turtle-monotile-kites-fresnel.svg's viewBox. These
// two stay in sync by construction rather than by discipline: the generator
// derives its placements from the very kite paths that file draws.
const TILE_W = 5.9;
const TILE_H = 4.33012702;

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
    >
      {TURTLE_FIELD_PLACEMENTS.map((t, i) => (
        <image
          key={i}
          href="/turtle-monotile-kites-fresnel.svg"
          width={TILE_W}
          height={TILE_H}
          // SVG applies transforms right to left, so this is mirror, then
          // rotate, then translate: the same p' = R(rot) . S(mirrored) . p
          // order projectTile uses in tunnelSpiral.ts. The artwork's origin is
          // the turtle's V0, which is the pivot both that convention and the
          // generator rotate about, so no pivot offset is needed here.
          transform={
            `translate(${t.x},${t.y}) rotate(${t.rot})` +
            (t.mirrored ? " scale(-1,1)" : "")
          }
        />
      ))}
    </svg>
  );
}
