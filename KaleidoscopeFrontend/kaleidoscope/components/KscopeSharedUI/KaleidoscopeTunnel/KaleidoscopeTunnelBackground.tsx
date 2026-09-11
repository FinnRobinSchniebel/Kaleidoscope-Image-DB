"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { TUNNEL_TILE_PATCH } from "./tunnelTilePatch.ts";
import { projectTunnelTiles, TILE_LOCAL_STROKE_WIDTH, type ConeConfig } from "./tunnelSpiral.ts";
import TurtleFieldBackground from "./TurtleFieldBackground.tsx";

const DEFAULT_PALETTE = [
  "#15dcff", "#f3f5f8", "#37237a", "#4478f1", "#ff9cf0", "#9cf3fa","#4478f1", "#9cf3fa", "#ffac9c", "#a4fcce"
];

const PALETTE_GLASS = [
  "#cce0ff", "#e0fbff", "#e8fcf5", "#f9fce8", "#fce9e8", "#fbe8fc", "#f2e8fc"
]

// Fixed screen-px offset each tile's side wall is extruded toward -- same
// vector for every tile regardless of depth, like a single light source
// rather than a true per-tile 3D normal.
const EXTRUDE = { dx: -3, dy: 3 } as const;

function shiftPoints(points: string, dx: number, dy: number): string {
  return points
    .split(" ")
    .map((pair) => {
      const [x, y] = pair.split(",").map(Number);
      return `${x + dx},${y + dy}`;
    })
    .join(" ");
}

// Multiplier on TILE_LOCAL_STROKE_WIDTH -- the rim-light knob.
// Below 1 narrows the lit band toward the edge; above 1 widens it,
// eating further into each tile's interior.
const RIM_WIDTH_SCALE = .7;

// One small bump, zero at its own edges so it tiles seamlessly via
// feTile below. Generic and independent of tile geometry, so it's
// generated once on mount rather than reacting to tiles/phase.
function generateBumpTile(tileSize: number): string {
  const canvas = document.createElement("canvas");
  canvas.width = tileSize;
  canvas.height = tileSize;
  const ctx = canvas.getContext("2d")!;
  const imageData = ctx.createImageData(tileSize, tileSize);
  const data = imageData.data;
  const center = tileSize / 2;
  const maxR = tileSize / 2;
  for (let y = 0; y < tileSize; y++) {
    for (let x = 0; x < tileSize; x++) {
      const ox = x - center;
      const oy = y - center;
      const r = Math.sqrt(ox * ox + oy * oy);
      const t = Math.min(1, r / maxR);
      const magnitude = Math.sin(Math.PI * t);
      const invLen = r > 0 ? magnitude / r : 0;
      const dx = ox * invLen;
      const dy = oy * invLen;
      const i = (y * tileSize + x) * 4;
      data[i] = Math.max(0, Math.min(255, 128 + dx * 127));
      data[i + 1] = Math.max(0, Math.min(255, 128 + dy * 127));
      data[i + 2] = 128;
      data[i + 3] = 255;
    }
  }
  ctx.putImageData(imageData, 0, 0);
  return canvas.toDataURL();
}

// Repeat spacing (px) for the bump pattern -- a visual knob, not
// derived from tile size.
const LENS_BUMP_TILE_PX = 80;

// Perspective every tile's projection (see tunnelSpiral.ts's projectTile)
// is computed under. Not a prop since it isn't a per-tile concern.
const PERSPECTIVE = 1800;

// Describes TUNNEL_TILE_PATCH's own data (see that file's header) -- not a
// visual knob. Its (u, v) values and rot/mirrored placements only make
// sense at this scale; changing these without regenerating the patch will
// make every tile's position and size wrong.
const PATCH_SHAPE = {
  uMin: -120,
  uMax: 124,
  uvScale: 38.709678,
} as const;

// Visual tuning knobs -- each is independent and clamped below to a range
// that can't break the render. Defaults match how this looked before these
// became props, so `<KaleidoscopeTunnelBackground />` with no props is
// unchanged.
export type KaleidoscopeTunnelBackgroundProps = {
  /** Full revolutions the spiral makes between its wide mouth and its tip. */
  turns?: number;

  /**
   * Width (diameter) of the cone's base/mouth, as a multiple of the
   * container's larger dimension (like a CSS vmax, but of the container,
   * not the viewport) - not raw px, so it stays proportional across screen
   * sizes. 1 spans that dimension edge to edge; default 1.8 deliberately
   * overshoots so the mouth starts off-screen. Lower toward/below 1 to
   * bring it on-screen, raise to push more off.
   */
  baseWidth?: number;

  /**
   * Depth (px) of the mouth from the PERSPECTIVE origin. 0 sits at that
   * origin, where a tile's rendered size matches its real px size;
   * negative moves the mouth away (smaller). Clamped below PERSPECTIVE to
   * avoid putting the mouth behind the camera.
   */
  startDepth?: number;

  /** How far (px) the tunnel recedes from its base to its tip. */
  depth?: number;

  /**
   * 0..1, blends which direction each tile is placed/sized across the
   * strip (see frameAt in tunnelSpiral.ts): 0 keeps tiles face-on but
   * leaves gaps between neighbors (worse near the wide mouth); 1 glues
   * neighbors together but tilts tiles into depth, away from the camera.
   * If tiles look too twisted at a value that also closes the gaps, widen
   * or shorten the cone (raise baseWidth, lower depth/turns) rather than
   * raising this further.
   */
  slantWeight?: number;

  //Screen position vanishing point
  tipFocusX?: number;
  tipFocusY?: number;

  //Screen position base center point (center of funnel start)
  baseFocusX?: number;
  baseFocusY?: number;

  //Seconds per rotation
  rotationPeriod?: number;

  //List of tile colors (any valid css color)
  palette?: readonly string[];
};

export default function KaleidoscopeTunnelBackground({
  turns = 2.5,
  baseWidth = 1.3,
  startDepth: startDepthProp = 0,
  depth = 9000,
  slantWeight: slantWeightProp = 0.0,
  tipFocusX = 0.95,
  tipFocusY = 0.2,
  baseFocusX = -1,
  baseFocusY = 1.9,
  rotationPeriod = 1200,
  palette = PALETTE_GLASS,
}: KaleidoscopeTunnelBackgroundProps = {}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState<{ w: number; h: number } | null>(null);
  const [phase, setPhase] = useState(0);
  const [lensMapUrl, setLensMapUrl] = useState<string | null>(null);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const update = () => {
      const rect = el.getBoundingClientRect();
      setSize({ w: rect.width, h: rect.height });
    };
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  useEffect(() => {
    const period = Math.max(1, rotationPeriod);
    let raf = 0;
    let start: number | null = null;
    let lastUpdate = -Infinity;
    // Derives phase from elapsed wall-clock time rather than accumulating
    // a fixed per-tick step, so it stays accurate regardless of which
    // frames get skipped below. ~20fps reads as smooth for a rotation
    // this slow, at a third of full rAF's re-projection cost.
    const tick = (now: number) => {
      if (start === null) start = now;
      if (now - lastUpdate >= 50) {
        lastUpdate = now;
        const elapsed = (now - start) / 1000;
        setPhase(((2 * Math.PI * elapsed) / period) % (2 * Math.PI));
      }
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [rotationPeriod]);

  const tiles = useMemo(() => {
    if (size === null) return null;
    const startDepth = Math.min(startDepthProp, PERSPECTIVE - 100);
    // Mouth-to-tip world offset that makes the mouth's screen projection
    // land on baseFocus while the tip (unaffected, since taper zeroes
    // this out there) stays on tipFocus.
    const pfNear = PERSPECTIVE / (PERSPECTIVE - startDepth);
    const baseOffset: readonly [number, number] = [
      (size.w * (baseFocusX - tipFocusX)) / pfNear,
      (size.h * (baseFocusY - tipFocusY)) / pfNear,
    ];
    const cfg: ConeConfig = {
      ...PATCH_SHAPE,
      turns,
      slantWeight: Math.min(1, Math.max(0, slantWeightProp)),
      rNear: (Math.max(0.01, baseWidth) * Math.max(size.w, size.h)) / 2,
      zNear: startDepth,
      zFar: startDepth - Math.max(1, depth),
      baseOffset,
      phase,
    };
    const projected = projectTunnelTiles(TUNNEL_TILE_PATCH, cfg, PERSPECTIVE);
    // No 3D compositor sorts depth for us (see projectTile); sort by z
    // ascending so nearer tiles paint over farther ones.
    return projected
      .map((p, i) => {
        const paletteIndex = i % palette.length;
        // Stable across re-sorts (unlike the post-sort array index), so
        // React can match tiles to their existing DOM nodes across phase
        // ticks instead of tearing them down -- z (and so paint order)
        // rotates with phase since Gorth feeds into P[2].
        return { ...p, id: i, color: palette[paletteIndex], paletteIndex };
      })
      .sort((a, b) => a.z - b.z);
  }, [size, phase, startDepthProp, baseFocusX, tipFocusX, baseFocusY, tipFocusY, turns, slantWeightProp, baseWidth, depth, palette]);

  useEffect(() => {
    setLensMapUrl(generateBumpTile(LENS_BUMP_TILE_PX));
  }, []);

  return (
    <div
      ref={containerRef}
      className="fixed inset-0 -z-10 w-screen overflow-hidden pointer-events-none "
      style={{
        // Tailwind's bg-radial-* utilities can't take a dynamic position
        // (tipFocus isn't known until build/runtime), so this gradient is
        // plain CSS instead -- kept in sync with the tunnel's own vanishing
        // point so the glow sits right where the tiles converge.
        backgroundImage: `radial-gradient(circle at ${tipFocusX * 100}% ${tipFocusY * 95}% in oklab, white 1%, var(--color-teal-200) 7%, #77c2ff 90%)`,
      }}
    >
      {/* Middle layer: static turtle-monotile texture over the gradient,
          under the spiral. Z-order comes from document order alone -- both
          this and the spiral are absolutely positioned siblings, so no
          z-index is involved. */}
      <TurtleFieldBackground />

      {/* Sizing depends on measuring this element, which only exists once
          mounted in the browser -- rendering tiles only after that avoids
          a spurious hydration diff against SSR's guessed-size markup. */}
      {tiles !== null && size !== null && lensMapUrl !== null && (
        <>
          {/* Warps only the background pixels sitting behind the tiles'
              own footprint (via the SVG mask below), instead of the whole
              screen -- backdrop-filter on the tile svg itself would apply
              to its entire bounding box, including gaps between tiles. */}
          <div
            className="absolute inset-0"
            style={{
              backdropFilter: "url(#lensFilter)",
              WebkitBackdropFilter: "url(#lensFilter)",
              maskImage: "url(#tilesMask)",
              WebkitMaskImage: "url(#tilesMask)",
            }}
          />
          <svg width={size.w} height={size.h} className="absolute inset-0 stroke-primary/30 stroke-2">
            <defs>
              {palette.map((color, i) => (
                <linearGradient key={i} id={`tileGrad-${i}`} x1="0%" y1="0%" x2="100%" y2="100%">
                  <stop offset="0%" stopColor={`color-mix(in oklab, ${color} 100%, white 35%)`} />
                  <stop offset="100%" stopColor={`color-mix(in oklab, ${color} 100%, black 15%)`} />
                </linearGradient>
              ))}
              <mask id="tilesMask" maskUnits="userSpaceOnUse" x="0" y="0" width={size.w} height={size.h}>
                <g transform={`translate(${size.w * tipFocusX},${size.h * tipFocusY})`}>
                  {tiles.map((t) => (
                    <g key={t.id}>
                      <polygon points={shiftPoints(t.points, EXTRUDE.dx, EXTRUDE.dy)} fill="white" />
                      <polygon points={t.points} fill="white" />
                    </g>
                  ))}
                </g>
              </mask>
              {/* Single feDisplacementMap pass fed by one small bump
                  repeated via feTile: many little lenses at roughly tile
                  scale, not one lens per real tile, and won't align to
                  each tile's actual boundary (see generateBumpTile). */}
              <filter id="lensFilter" colorInterpolationFilters="sRGB">
                <feImage href={lensMapUrl} x="0" y="0" width={LENS_BUMP_TILE_PX} height={LENS_BUMP_TILE_PX} result="bumpTile" />
                <feTile in="bumpTile" result="lensMap" />
                <feDisplacementMap in="SourceGraphic" in2="lensMap" scale={40} xChannelSelector="R" yChannelSelector="G" />
              </filter>
              {/* Explicit region: the default (-10%/120% of the stroked
                  shape's own bbox) was clipping the blurred stroke away
                  almost entirely, since strokeWidth is often a large
                  fraction of that bbox. */}
              <filter id="tileRimBlur" x="-100%" y="-100%" width="300%" height="300%">
                <feGaussianBlur stdDeviation={3} />
              </filter>
              {/* Fades each tile toward transparent at center, opaque at
                  the edge. Traces the real (spiky, concave) boundary via
                  a stroke rather than a circular gradient, which would
                  only reach full strength near the single farthest
                  vertex. */}
              {tiles.map((t) => (
                <mask key={t.id} id={`tileRimMask-${t.id}`} x="-50%" y="-50%" width="200%" height="200%">
                  <polygon points={t.points} fill="white" fillOpacity={0.25} />
                  <polygon
                    points={t.points}
                    fill="none"
                    stroke="white"
                    strokeWidth={Math.max(2, TILE_LOCAL_STROKE_WIDTH * t.scale * RIM_WIDTH_SCALE)}
                    filter="url(#tileRimBlur)"
                  />
                </mask>
              ))}
            </defs>
            <g transform={`translate(${size.w * tipFocusX},${size.h * tipFocusY})`}>
              {tiles.map((t) => (
                <g key={t.id}>
                  <polygon points={shiftPoints(t.points, EXTRUDE.dx, EXTRUDE.dy)} fill={`color-mix(in oklab, ${t.color} 55%, black)`}  mask={`url(#tileRimMask-${t.id})`}/>
                  <polygon points={t.points} fill={`url(#tileGrad-${t.paletteIndex})`} mask={`url(#tileRimMask-${t.id})`} />
                </g>
              ))}
            </g>
          </svg>
        </>
      )}
    </div>
  );
}
