"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { TUNNEL_TILE_PATCH } from "./tunnelTilePatch.ts";
import { computeTunnelTransforms, type ConeConfig } from "./tunnelSpiral.ts";

const PALETTE = [
  "#6d5acd", "#5a4fcf", "#8a5ccf", "#4a3fae", "#b45cc9",
  "#7c6fe0", "#c9c9d8", "#8f8fd0", "#a75ccf", "#5c5cae",
];

// The CSS `perspective` the whole scene renders under (also used below to
// keep `startDepth` from crossing it). Changing this changes how strongly
// depth is foreshortened; it isn't a per-tile concern so it isn't part of
// TUNNEL_SETTINGS.
const PERSPECTIVE = 1800;

// Describes TUNNEL_TILE_PATCH's own data (see that file's header) -- not a
// visual knob. Its (u, v) values and rot/mirrored placements only make
// sense at this scale; changing these without regenerating the patch will
// make every tile's position and size wrong.
const PATCH_SHAPE = {
  uMin: -120,
  uMax: 120,
  uvScale: 38.709678,
} as const;

// Visual tuning knobs -- edit these freely, each is independent and
// clamped below to a range that can't break the render.
const TUNNEL_SETTINGS = {
  // Full revolutions the spiral makes between its wide mouth and its tip.
  turns: 2,

  // Width (diameter) of the cone's base/mouth, in screen units -- a
  // multiple of the container's own larger dimension (like a CSS `vmax`,
  // but of the container, not the viewport; see the ResizeObserver below),
  // not a raw px value, so it stays proportional to the screen on any
  // device instead of being too small on a large monitor or spilling off a
  // tiny one. 1 = exactly spans the container's larger dimension edge to
  // edge; the default of 1.8 deliberately overshoots it so the mouth
  // reliably starts outside the visible viewport on any screen size (per
  // the original "starts from outside the screen" design goal) -- lower
  // it toward/below 1 to bring the mouth on-screen, raise it to push more
  // of it off.
  baseWidth: 1.2,

  // Depth (px) of the mouth, measured from the CSS `perspective` origin
  // below. 0 sits right at that origin, where a tile's rendered size
  // exactly matches its real px size; negative pushes the mouth further
  // away (smaller, more foreshortened). Clamped away from `PERSPECTIVE`
  // itself, which would otherwise put the mouth behind the camera.
  startDepth: 0,

  // How far (px) the tunnel recedes from its mouth to its tip.
  depth: 9000,

  // 0..1. Picks which direction each tile is placed and sized along
  // across the strip (see frameAt's comment in tunnelSpiral.ts), blending
  // between two failure modes that trade off against each other -- no
  // value avoids both:
  //   0 keeps tiles face-on (their full hat shape stays visible) but lets
  //     neighboring tiles' edges drift apart -- worse toward the wide
  //     mouth, where tiles are biggest.
  //   1 keeps every tile glued edge-to-edge to its neighbors, but does it
  //     by tilting tiles into depth -- visually, tiles twist away from the
  //     camera toward the tip/vanishing point instead of facing you,
  //     worse the higher this goes.
  // If tiles look too twisted at a value that also closes the gaps, try
  // widening/shortening the cone first (raise baseWidth, lower depth, or
  // lower turns) rather than pushing this further -- a shallower cone
  // needs less of this blend either way.
  slantWeight: 0.3,
};

export default function KaleidoscopeTunnelBackground() {
  const containerRef = useRef<HTMLDivElement>(null);
  const [screenUnit, setScreenUnit] = useState<number | null>(null);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const update = () => {
      const rect = el.getBoundingClientRect();
      setScreenUnit(Math.max(rect.width, rect.height));
    };
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const transforms = useMemo(() => {
    if (screenUnit === null) return null;
    const startDepth = Math.min(TUNNEL_SETTINGS.startDepth, PERSPECTIVE - 100);
    const cfg: ConeConfig = {
      ...PATCH_SHAPE,
      turns: TUNNEL_SETTINGS.turns,
      slantWeight: Math.min(1, Math.max(0, TUNNEL_SETTINGS.slantWeight)),
      rNear: (Math.max(0.01, TUNNEL_SETTINGS.baseWidth) * screenUnit) / 2,
      zNear: startDepth,
      zFar: startDepth - Math.max(1, TUNNEL_SETTINGS.depth),
    };
    return computeTunnelTransforms(TUNNEL_TILE_PATCH, cfg);
  }, [screenUnit]);

  return (
    <div
      ref={containerRef}
      className="fixed inset-0 -z-10 overflow-hidden pointer-events-none flex items-center justify-center bg-black"
    >
      {/* baseWidth is relative to this element's own measured size, which
          only exists once mounted in the browser -- rendering tiles only
          after that avoids a spurious hydration diff against SSR's
          guessed-size markup. */}
      {transforms !== null && (
        <div className="relative w-full h-full flex items-center justify-center" style={{ perspective: PERSPECTIVE }}>
          <div className="relative" style={{ width: 0, height: 0, transformStyle: "preserve-3d" }}>
            {TUNNEL_TILE_PATCH.map((_, i) => (
              <div
                key={i}
                className="tunnel-tile"
                style={{
                  transform: transforms[i],
                  background: PALETTE[i % PALETTE.length],
                }}
              />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
