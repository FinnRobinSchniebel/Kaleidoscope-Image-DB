"use client";

import { useEffect, useRef } from "react";
import { useWebGLCanvas } from "@/webgl/core/useWebGLCanvas.ts";
import { useRenderLoop } from "@/webgl/core/RenderLoop.ts";
import { TunnelScene } from "./TunnelScene.ts";

// Same shape as KaleidoscopeTunnelBackgroundProps in the SVG version
// (components/KscopeSharedUI/KaleidoscopeTunnel/KaleidoscopeTunnelBackground.tsx)
// so TunnelBackground.tsx can forward the same props to either
// implementation. Most fields aren't consumed here yet -- wired in as
// M3/M4 land.
export type TunnelBackgroundProps = {
  turns?: number;
  baseWidth?: number;
  startDepth?: number;
  depth?: number;
  slantWeight?: number;
  tipFocusX?: number;
  tipFocusY?: number;
  baseFocusX?: number;
  baseFocusY?: number;
  rotationPeriod?: number;
  palette?: readonly string[];
};

// More colors than the SVG version's own default palette specifically to
// keep same-color adjacent tiles rare during dev inspection -- palette[i %
// length] cycles by raw patch-array index, not spatial position, so with
// too few colors a real (if slightly uneven) gap between same-colored
// neighbors can misread as a crack through what looks like one tile.
const DEFAULT_PALETTE = ["#ff6b6b", "#4dabf7", "#69db7c", "#ffd43b", "#da77f2", "#ff922b", "#20c997", "#748ffc"];

// M3: all patch tiles instanced along the cone/spiral wrap, palette-
// colored, phase fixed (no rotation animation yet) -- see TunnelScene.ts.
export default function KaleidoscopeTunnelBackgroundGL({
  palette = DEFAULT_PALETTE,
}: TunnelBackgroundProps = {}) {
  const { containerRef, canvasRef, renderer, size } = useWebGLCanvas();
  const sceneRef = useRef<TunnelScene | null>(null);

  useEffect(() => {
    if (!renderer) return;
    const tunnel = new TunnelScene();
    let cancelled = false;
    tunnel.load(palette).then(() => {
      if (!cancelled) sceneRef.current = tunnel;
    });
    return () => {
      cancelled = true;
      tunnel.dispose();
      sceneRef.current = null;
    };
  }, [renderer, palette]);

  useRenderLoop(23, () => {
    const tunnel = sceneRef.current;
    if (!renderer || !tunnel || !size) return;
    tunnel.setSize(size.width, size.height);
    tunnel.render(renderer);
  });

  return (
    <div ref={containerRef} className="fixed inset-0 -z-10 w-screen overflow-hidden pointer-events-none">
      <canvas ref={canvasRef} className="absolute inset-0 h-full w-full" />
    </div>
  );
}
