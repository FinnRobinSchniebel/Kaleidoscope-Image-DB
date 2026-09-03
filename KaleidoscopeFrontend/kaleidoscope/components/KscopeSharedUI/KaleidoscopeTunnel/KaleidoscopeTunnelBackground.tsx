"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { TUNNEL_TILE_PATCH } from "./tunnelTilePatch.ts";
import { projectTunnelTiles, type ConeConfig } from "./tunnelSpiral.ts";

const PALETTE = [
  "#15dcff", "var(--color-white)", "var(--color-blue-50)", "var(--color-blue-300)", "#425df7", "var(--color-teal-200)"
];

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

// Perspective every tile's projection (see tunnelSpiral.ts's projectTile)
// is computed under. Not part of TUNNEL_SETTINGS since it isn't a per-tile
// concern.
const PERSPECTIVE = 1800;

// Describes TUNNEL_TILE_PATCH's own data (see that file's header) -- not a
// visual knob. Its (u, v) values and rot/mirrored placements only make
// sense at this scale; changing these without regenerating the patch will
// make every tile's position and size wrong.
const PATCH_SHAPE = {
  uMin: -120,
  uMax: 121,
  uvScale: 38.709678,
} as const;

// Visual tuning knobs -- edit these freely, each is independent and
// clamped below to a range that can't break the render.
const TUNNEL_SETTINGS = {
  // Full revolutions the spiral makes between its wide mouth and its tip.
  turns: 2.5,

  // Width (diameter) of the cone's base/mouth, as a multiple of the
  // container's larger dimension (like a CSS vmax, but of the container,
  // not the viewport) - not raw px, so it stays proportional across screen
  // sizes. 1 spans that dimension edge to edge; default 1.8 deliberately
  // overshoots so the mouth starts off-screen. Lower toward/below 1 to
  // bring it on-screen, raise to push more off.
  baseWidth: 1.3,

  // Depth (px) of the mouth from the PERSPECTIVE origin. 0 sits at that
  // origin, where a tile's rendered size matches its real px size;
  // negative moves the mouth away (smaller). Clamped below PERSPECTIVE to
  // avoid putting the mouth behind the camera.
  startDepth: 0,

  // How far (px) the tunnel recedes from its mouth to its tip.
  depth: 9000,

  // 0..1, blends which direction each tile is placed/sized across the
  // strip (see frameAt in tunnelSpiral.ts): 0 keeps tiles face-on but
  // leaves gaps between neighbors (worse near the wide mouth); 1 glues
  // neighbors together but tilts tiles into depth, away from the camera.
  // If tiles look too twisted at a value that also closes the gaps, widen
  // or shorten the cone (raise baseWidth, lower depth/turns) rather than
  // raising this further.
  slantWeight: 0.0,

  // Where the tunnel's vanishing point (tip) sits, as a fraction of the
  // container's width/height (0.5, 0.5 -- the default -- is dead center).
  tipFocusX: 0.95,
  tipFocusY: 0.2,

  // Where the mouth's center sits, same units as tipFocus*. Equal to
  // tipFocus* (the default) keeps the tunnel aimed straight at the
  // viewer; diverging the two tilts its axis so it points elsewhere.
  baseFocusX: -1,
  baseFocusY: 1.9,

  // Seconds for one full spin around the tunnel's own axis.
  rotationPeriod: 1200,
};


export default function KaleidoscopeTunnelBackground() {
  const containerRef = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState<{ w: number; h: number } | null>(null);
  const [phase, setPhase] = useState(0);

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
    const period = Math.max(1, TUNNEL_SETTINGS.rotationPeriod);
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
  }, []);

  const tiles = useMemo(() => {
    if (size === null) return null;
    const startDepth = Math.min(TUNNEL_SETTINGS.startDepth, PERSPECTIVE - 100);
    // Mouth-to-tip world offset that makes the mouth's screen projection
    // land on baseFocus while the tip (unaffected, since taper zeroes
    // this out there) stays on tipFocus.
    const pfNear = PERSPECTIVE / (PERSPECTIVE - startDepth);
    const baseOffset: readonly [number, number] = [
      (size.w * (TUNNEL_SETTINGS.baseFocusX - TUNNEL_SETTINGS.tipFocusX)) / pfNear,
      (size.h * (TUNNEL_SETTINGS.baseFocusY - TUNNEL_SETTINGS.tipFocusY)) / pfNear,
    ];
    const cfg: ConeConfig = {
      ...PATCH_SHAPE,
      turns: TUNNEL_SETTINGS.turns,
      slantWeight: Math.min(1, Math.max(0, TUNNEL_SETTINGS.slantWeight)),
      rNear: (Math.max(0.01, TUNNEL_SETTINGS.baseWidth) * Math.max(size.w, size.h)) / 2,
      zNear: startDepth,
      zFar: startDepth - Math.max(1, TUNNEL_SETTINGS.depth),
      baseOffset,
      phase,
    };
    const projected = projectTunnelTiles(TUNNEL_TILE_PATCH, cfg, PERSPECTIVE);
    // No 3D compositor sorts depth for us (see projectTile); sort by z
    // ascending so nearer tiles paint over farther ones.
    return projected
      .map((p, i) => {
        const paletteIndex = i % PALETTE.length;
        // Stable across re-sorts (unlike the post-sort array index), so
        // React can match tiles to their existing DOM nodes across phase
        // ticks instead of tearing them down -- z (and so paint order)
        // rotates with phase since Gorth feeds into P[2].
        return { ...p, id: i, color: PALETTE[paletteIndex], paletteIndex };
      })
      .sort((a, b) => a.z - b.z);
  }, [size, phase]);

  return (
    <div
      ref={containerRef}
      className="fixed inset-0 -z-10 w-screen overflow-hidden pointer-events-none"
      style={{
        // Tailwind's bg-radial-* utilities can't take a dynamic position
        // (tipFocus isn't known until build/runtime), so this gradient is
        // plain CSS instead -- kept in sync with the tunnel's own vanishing
        // point so the glow sits right where the tiles converge.
        backgroundImage: `radial-gradient(circle at ${TUNNEL_SETTINGS.tipFocusX * 95}% ${TUNNEL_SETTINGS.tipFocusY * 95}% in oklab, white 1%, var(--color-teal-200) 30%, #77c2ff)`,
      }}
    >
      {/* Sizing depends on measuring this element, which only exists once
          mounted in the browser -- rendering tiles only after that avoids
          a spurious hydration diff against SSR's guessed-size markup. */}
      {tiles !== null && size !== null && (
        <svg width={size.w} height={size.h} className="absolute inset-0 stroke-primary/40 stroke-2 ">
          <defs>
            {PALETTE.map((color, i) => (
              <linearGradient key={i} id={`tileGrad-${i}`} x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" stopColor={`color-mix(in oklab, ${color} 100%, white 35%)`} />
                <stop offset="100%" stopColor={`color-mix(in oklab, ${color} 100%, black 15%)`} />
              </linearGradient>
            ))}
          </defs>
          <g transform={`translate(${size.w * TUNNEL_SETTINGS.tipFocusX},${size.h * TUNNEL_SETTINGS.tipFocusY})`}>
            {tiles.map((t) => (
              <g key={t.id}>
                <polygon points={shiftPoints(t.points, EXTRUDE.dx, EXTRUDE.dy)} fill={`color-mix(in oklab, ${t.color} 65%, black)`} fillOpacity={.5}
                />
                <polygon points={t.points} fill={`url(#tileGrad-${t.paletteIndex})`} fillOpacity={.7} />
              </g>
            ))}
          </g>
        </svg>
      )}
    </div>
  );
}
