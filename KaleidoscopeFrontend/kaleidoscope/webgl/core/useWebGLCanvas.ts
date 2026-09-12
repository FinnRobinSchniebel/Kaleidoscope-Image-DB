"use client";

import { useEffect, useRef, useState } from "react";
import * as THREE from "three";

export type CanvasSize = { width: number; height: number };

// Mounts a THREE.WebGLRenderer against a <canvas>, sized via ResizeObserver
// against a separate wrapping container (not the canvas itself, to avoid
// the canvas measuring its own not-yet-set size), and disposes the renderer
// on unmount.
export function useWebGLCanvas() {
  const containerRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [renderer, setRenderer] = useState<THREE.WebGLRenderer | null>(null);
  const [size, setSize] = useState<CanvasSize | null>(null);

  useEffect(() => {
    const container = containerRef.current;
    const canvas = canvasRef.current;
    if (!container || !canvas) return;

    const gl = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true });
    // Caps DPR on high-density mobile screens -- fillrate cost grows with
    // the square of pixel ratio, and this is a background element.
    gl.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    setRenderer(gl);

    const update = () => {
      const rect = container.getBoundingClientRect();
      gl.setSize(rect.width, rect.height, false);
      setSize({ width: rect.width, height: rect.height });
    };
    update();
    const ro = new ResizeObserver(update);
    ro.observe(container);

    return () => {
      ro.disconnect();
      gl.dispose();
      setRenderer(null);
    };
  }, []);

  return { containerRef, canvasRef, renderer, size };
}
