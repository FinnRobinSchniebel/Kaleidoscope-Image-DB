"use client";

import { useEffect, useRef } from "react";

// Derives elapsed time from the wall clock each tick rather than an
// accumulated step, so it can't drift regardless of which frames get
// skipped -- same pattern the SVG tunnel's own phase loop uses.
export function useRenderLoop(fps: number, onTick: (elapsedSeconds: number) => void): void {
  // Ref, not a dependency: onTick is typically a fresh closure every render,
  // and restarting the loop on every render would reset `start` and make
  // elapsed time jump backward.
  const onTickRef = useRef(onTick);
  onTickRef.current = onTick;

  useEffect(() => {
    const interval = 1000 / Math.max(1, fps);
    let raf = 0;
    let start: number | null = null;
    let lastUpdate = -Infinity;
    const tick = (now: number) => {
      if (start === null) start = now;
      if (now - lastUpdate >= interval) {
        lastUpdate = now;
        onTickRef.current((now - start) / 1000);
      }
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [fps]);
}
