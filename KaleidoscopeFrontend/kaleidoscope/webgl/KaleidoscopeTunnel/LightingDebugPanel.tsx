"use client";

import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import type { TunnelLighting, TunnelMaterial } from "./TunnelScene.ts";

// Dev-only tool for tuning TunnelScene's lighting/material by eye. No
// light/dark-theme split: next-themes is a dependency but has no
// ThemeProvider wired up anywhere in this app, so there's no real "current
// theme" to read yet -- wiring that up is a separate, larger change. One
// preset each, tunable live here when either needs revisiting.
type Field<T> = { key: keyof T; label: string; max: number; step: number };

// keyIntensity's range is ~1000x the other two, not a typo -- see
// DEFAULT_LIGHTING's comment in TunnelScene.ts: its point light's distance
// falloff makes small values numerically invisible at this scene's scale.
const LIGHTING_FIELDS: Field<TunnelLighting>[] = [
  { key: "ambientIntensity", label: "Ambient (floor)", max: 6, step: 0.05 },
  { key: "keyIntensity", label: "Key (at tip)", max: 20000, step: 50 },
  { key: "fillIntensity", label: "Fill (at camera)", max: 6, step: 0.05 },
];

const MATERIAL_FIELDS: Field<TunnelMaterial>[] = [
  { key: "roughness", label: "Roughness (+clearcoat)", max: 1, step: 0.01 },
  { key: "metalness", label: "Metalness", max: 1, step: 0.01 },
  { key: "envMapIntensity", label: "Env reflection", max: 2, step: 0.01 },
  { key: "clearcoat", label: "Clearcoat", max: 1, step: 0.01 },
];

function SliderSection<T extends Record<string, number>>({
  title,
  value,
  onChange,
  fields,
}: {
  title: string;
  value: T;
  onChange: (next: T) => void;
  fields: Field<T>[];
}) {
  return (
    <div className="mb-3 last:mb-0">
      <div className="mb-2 font-bold">{title}</div>
      {fields.map(({ key, label, max, step }) => (
        <label key={String(key)} className="mb-2 block">
          <div className="mb-0.5 flex justify-between">
            <span>{label}</span>
            <span>{value[key].toFixed(step < 1 ? 2 : 0)}</span>
          </div>
          <input
            type="range"
            min={0}
            max={max}
            step={step}
            value={value[key]}
            onChange={(e) => onChange({ ...value, [key]: Number(e.target.value) })}
            className="w-full"
          />
        </label>
      ))}
    </div>
  );
}

export default function LightingDebugPanel({
  lighting,
  onLightingChange,
  material,
  onMaterialChange,
}: {
  lighting: TunnelLighting;
  onLightingChange: (next: TunnelLighting) => void;
  material: TunnelMaterial;
  onMaterialChange: (next: TunnelMaterial) => void;
}) {
  // Portaled to <body>, not rendered in place: the tunnel's own wrapper div
  // is -z-10 (it sits behind page content), and a negative-z-index ancestor
  // caps its descendants' stacking order below the rest of the page in
  // hit-test/paint order regardless of the z-index this panel sets on
  // itself. Portaling to <body> escapes that stacking context entirely.
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);
  if (!mounted) return null;

  return createPortal(
    <div className="pointer-events-auto fixed top-4 right-4 z-50 w-64 rounded-md bg-black/70 p-3 font-mono text-xs text-white backdrop-blur-sm">
      <SliderSection title="Tunnel lighting (dev)" value={lighting} onChange={onLightingChange} fields={LIGHTING_FIELDS} />
      <SliderSection title="Tunnel material (dev)" value={material} onChange={onMaterialChange} fields={MATERIAL_FIELDS} />
    </div>,
    document.body,
  );
}
