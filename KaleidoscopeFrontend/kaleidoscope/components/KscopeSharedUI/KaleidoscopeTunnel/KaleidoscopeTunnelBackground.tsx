"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { TUNNEL_TILE_PATCH } from "./tunnelTilePatch.ts";
import { projectTunnelTiles, type ConeConfig } from "./tunnelSpiral.ts";

const PALETTE = [
  "#15dcff", "var(--color-white)", "var(--color-blue-50)", "var(--color-blue-300)", "#425df7", "var(--color-teal-200)" 
];

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
  turns: 2,

  // Width (diameter) of the cone's base/mouth, as a multiple of the
  // container's larger dimension (like a CSS vmax, but of the container,
  // not the viewport) - not raw px, so it stays proportional across screen
  // sizes. 1 spans that dimension edge to edge; default 1.8 deliberately
  // overshoots so the mouth starts off-screen. Lower toward/below 1 to
  // bring it on-screen, raise to push more off.
  baseWidth: 1.8,

  // Depth (px) of the mouth from the PERSPECTIVE origin. 0 sits at that
  // origin, where a tile's rendered size matches its real px size;
  // negative moves the mouth away (smaller). Clamped below PERSPECTIVE to
  // avoid putting the mouth behind the camera.
  startDepth: 0,

  // How far (px) the tunnel recedes from its mouth to its tip.
  depth: 6000,

  // 0..1, blends which direction each tile is placed/sized across the
  // strip (see frameAt in tunnelSpiral.ts): 0 keeps tiles face-on but
  // leaves gaps between neighbors (worse near the wide mouth); 1 glues
  // neighbors together but tilts tiles into depth, away from the camera.
  // If tiles look too twisted at a value that also closes the gaps, widen
  // or shorten the cone (raise baseWidth, lower depth/turns) rather than
  // raising this further.
  slantWeight: 0.0,
};

export default function KaleidoscopeTunnelBackground() {
  const containerRef = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState<{ w: number; h: number } | null>(null);

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

  const tiles = useMemo(() => {
    if (size === null) return null;
    const startDepth = Math.min(TUNNEL_SETTINGS.startDepth, PERSPECTIVE - 100);
    const cfg: ConeConfig = {
      ...PATCH_SHAPE,
      turns: TUNNEL_SETTINGS.turns,
      slantWeight: Math.min(1, Math.max(0, TUNNEL_SETTINGS.slantWeight)),
      rNear: (Math.max(0.01, TUNNEL_SETTINGS.baseWidth) * Math.max(size.w, size.h)) / 2,
      zNear: startDepth,
      zFar: startDepth - Math.max(1, TUNNEL_SETTINGS.depth),
    };
    const projected = projectTunnelTiles(TUNNEL_TILE_PATCH, cfg, PERSPECTIVE);
    // No 3D compositor sorts depth for us (see projectTile); sort by z
    // ascending so nearer tiles paint over farther ones.
    return projected
      .map((p, i) => ({ ...p, color: PALETTE[i % PALETTE.length] }))
      .sort((a, b) => a.z - b.z);
  }, [size]);

  return (
    <div ref={containerRef} className="fixed inset-0 -z-10 w-screen overflow-hidden pointer-events-none bg-radial-[circle_in_oklab] from-white from-1% via-teal-200 via-30% to-[#77c2ff]">
      {/* Sizing depends on measuring this element, which only exists once
          mounted in the browser -- rendering tiles only after that avoids
          a spurious hydration diff against SSR's guessed-size markup. */}
      {tiles !== null && size !== null && (
        <svg width={size.w} height={size.h} className="absolute inset-0 stroke-primary/40 stroke-2 ">
          <g transform={`translate(${size.w / 2},${size.h / 2})`}>
            <g>
              <animateTransform
                attributeName="transform"
                type="rotate"
                from="0"
                to="360"
                dur="1200s"
                repeatCount="indefinite"
              />
              {tiles.map((t, i) => (
                <polygon key={i} points={t.points} fill={t.color} fillOpacity={.6}/>
              ))}
            </g>
          </g>
        </svg>
      )}
    </div>
  );
}
