"use client";

import { useEffect, useRef, useState } from "react";
import TurtleFieldBackground from "@/components/KscopeSharedUI/KaleidoscopeTunnel/TurtleFieldBackground.tsx";
import { useWebGLCanvas } from "@/webgl/core/useWebGLCanvas.ts";
import { useRenderLoop } from "@/webgl/core/RenderLoop.ts";
import { TunnelScene, DEFAULT_LIGHTING, DEFAULT_MATERIAL } from "./TunnelScene.ts";
import LightingDebugPanel from "./LightingDebugPanel.tsx";

// Same shape as KaleidoscopeTunnelBackgroundProps in the SVG version
// (components/KscopeSharedUI/KaleidoscopeTunnel/KaleidoscopeTunnelBackground.tsx),
// plus frameRate (a WebGL-only knob -- the SVG version's throttle is a fixed
// internal constant, not a prop), so TunnelBackground.tsx can forward the
// same props to either implementation.
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
  /** Render/re-layout rate cap, in frames per second. See the "Decisions
   * locked in" section of the migration plan for why this defaults well
   * below display refresh rate -- mobile battery/thermal cost for a
   * background element, not a technical ceiling. */
  frameRate?: number;
  palette?: readonly string[];
};

// Matches KaleidoscopeTunnelBackground.tsx's own PALETTE_GLASS -- neither
// real mount site (app/(app)/layout.tsx, app/(auth)/layout.tsx) passes a
// custom palette, so this default is what actually ships; it needs to match
// the SVG version's real look, not just exist as a placeholder.
const DEFAULT_PALETTE = ["#80b2ff", "#8ff0ff", "#91ffd9", "#edff87", "#9fd5fc", "#fcbfff", "#cdadff"];

// DEBUG ONLY -- flip to true to bring the live lighting/material tuning
// panel back; kept as a single flag (rather than deleting the panel) since
// it's the fastest way back in next time the look needs revisiting.
const SHOW_LIGHTING_PANEL = false;

// Phase is driven by elapsed wall-clock time (see tunnelSpiral.ts's own
// phase driver for why: derived from elapsed time rather than accumulated
// per-tick, so it can't drift regardless of which frames the throttle
// skips); every other cone-shape prop flows straight into
// TunnelScene.update() each tick. The turtle-field texture layer renders
// as a plain DOM/SVG sibling behind the transparent canvas -- it needs no
// WebGL work since nothing here warps or lights it.
export default function KaleidoscopeTunnelBackgroundGL({
  turns = 2.5,
  baseWidth = 1.3,
  startDepth = 0,
  depth = 9000,
  slantWeight = 0.0,
  tipFocusX = 0.95,
  tipFocusY = 0.2,
  baseFocusX = -1,
  baseFocusY = 1.9,
  rotationPeriod = 1200,
  frameRate = 23,
  palette = DEFAULT_PALETTE,
}: TunnelBackgroundProps = {}) {
  const { containerRef, canvasRef, renderer, size } = useWebGLCanvas();
  const sceneRef = useRef<TunnelScene | null>(null);
  const readyRef = useRef(false);
  const [lighting, setLighting] = useState(DEFAULT_LIGHTING);
  const [material, setMaterial] = useState(DEFAULT_MATERIAL);

  useEffect(() => {
    if (!renderer) return;
    // note: the canvas has no real frame in it until the async STL load
    // (tunnel.load()) finishes. Left alone, browsers can render that gap as
    // a flash of flat grey (an uninitialized drawing buffer) or, worse, an
    // opaque layer that paints above the rest of the page regardless of
    // -z-10 stacking. Clearing the buffer early does not fix this -- it
    // just replaces the flash with a visible pop when the clear itself
    // lands. Hiding the canvas via opacity until update()+render() has
    // actually run once sidesteps both failure modes at the source.
    readyRef.current = false;
    if (canvasRef.current) canvasRef.current.style.opacity = "0";
    const tunnel = new TunnelScene();
    let cancelled = false;
    tunnel.load(palette, material).then(() => {
      if (!cancelled) sceneRef.current = tunnel;
    });
    return () => {
      cancelled = true;
      tunnel.dispose();
      sceneRef.current = null;
    };
    // palette/material intentionally excluded -- a change to either
    // re-applies via the effects below instead of reloading geometry.
  }, [renderer]);

  useEffect(() => {
    sceneRef.current?.setPalette(palette);
  }, [palette]);

  useEffect(() => {
    sceneRef.current?.setMaterialSettings(material);
  }, [material]);

  useRenderLoop(frameRate, (elapsedSeconds) => {
    const tunnel = sceneRef.current;
    if (!renderer || !tunnel || !size) return;
    const period = Math.max(1, rotationPeriod);
    const phase = ((2 * Math.PI * elapsedSeconds) / period) % (2 * Math.PI);
    tunnel.update(
      { turns, baseWidth, startDepth, depth, slantWeight, tipFocusX, tipFocusY, baseFocusX, baseFocusY },
      lighting,
      phase,
      size.width,
      size.height,
    );
    tunnel.render(renderer);
    // Revealed only after a real frame exists -- see this effect's own
    // comment above. A direct style mutation (not React state) since this
    // runs every frame but only ever needs to actually change the DOM once.
    if (!readyRef.current) {
      readyRef.current = true;
      if (canvasRef.current) canvasRef.current.style.opacity = "1";
    }
  });

  return (
    <div
      ref={containerRef}
      className="fixed inset-0 -z-10 w-screen overflow-hidden pointer-events-none"
      style={{
        // Same gradient as the SVG version's own container, kept in sync
        // with the tunnel's own vanishing point (tipFocus) so the glow
        // sits where the tiles converge -- Tailwind's bg-radial-* utilities
        // can't take a dynamic position, hence plain CSS here too.
        backgroundImage: `radial-gradient(circle at ${tipFocusX * 100}% ${tipFocusY * 95}% in oklab, white 1%, var(--color-teal-200) 7%, #77c2ff 90%)`,
      }}
    >
      <TurtleFieldBackground />
      <canvas
        ref={canvasRef}
        className="absolute inset-0 h-full w-full opacity-0 transition-opacity duration-300"
      />
      {SHOW_LIGHTING_PANEL && process.env.NODE_ENV !== "production" && (
        <LightingDebugPanel
          lighting={lighting}
          onLightingChange={setLighting}
          material={material}
          onMaterialChange={setMaterial}
        />
      )}
    </div>
  );
}
