import { TUNNEL_TILE_PATCH } from "./tunnelTilePatch.ts";

const PALETTE = [
  "#6d5acd", "#5a4fcf", "#8a5ccf", "#4a3fae", "#b45cc9",
  "#7c6fe0", "#c9c9d8", "#8f8fd0", "#a75ccf", "#5c5cae",
];

export default function KaleidoscopeTunnelBackground() {
  return (
    <div className="absolute inset-0 -z-10 overflow-hidden pointer-events-none flex items-center justify-center bg-[#0a0a12]">
      <div className="relative w-full h-full">
        {TUNNEL_TILE_PATCH.map((t, i) => (
          <div
            key={i}
            className="tunnel-tile"
            style={{
              ["--u" as string]: t.u,
              ["--v" as string]: t.v,
              ["--rot" as string]: t.rot,
              ["--mirror" as string]: t.mirrored ? -1 : 1,
              background: PALETTE[i % PALETTE.length],
            }}
          />
        ))}
      </div>
    </div>
  );
}
