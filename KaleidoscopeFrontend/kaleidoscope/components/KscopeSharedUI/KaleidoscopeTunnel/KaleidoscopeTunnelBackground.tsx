"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { TUNNEL_TILE_PATCH } from "./tunnelTilePatch.ts";
import { computeTunnelTransforms, type ConeConfig } from "./tunnelSpiral.ts";

const PALETTE = [
  "#6d5acd", "#5a4fcf", "#8a5ccf", "#4a3fae", "#b45cc9",
  "#7c6fe0", "#c9c9d8", "#8f8fd0", "#a75ccf", "#5c5cae",
];

const BASE_CONFIG = {
  uMin: -120,
  uMax: 120,
  uvScale: 38.709678,
  turns: 3,
  zNear: 0,
  zFar: -9000,
  slantWeight: 0.4,
};

export default function KaleidoscopeTunnelBackground() {
  const containerRef = useRef<HTMLDivElement>(null);
  const [rNear, setRNear] = useState<number | null>(null);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const update = () => {
      const rect = el.getBoundingClientRect();
      setRNear(0.9 * Math.max(rect.width, rect.height));
    };
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const transforms = useMemo(() => {
    if (rNear === null) return null;
    const cfg: ConeConfig = { ...BASE_CONFIG, rNear };
    return computeTunnelTransforms(TUNNEL_TILE_PATCH, cfg);
  }, [rNear]);

  return (
    <div
      ref={containerRef}
      className="absolute inset-0 -z-10 overflow-hidden pointer-events-none flex items-center justify-center bg-black"
    >
      {/* rNear depends on measuring this element, which only exists once
          mounted in the browser -- rendering tiles only after that avoids
          a spurious hydration diff against SSR's guessed-size markup. */}
      {transforms !== null && (
        <div className="relative w-full h-full" style={{ perspective: 1800 }}>
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
