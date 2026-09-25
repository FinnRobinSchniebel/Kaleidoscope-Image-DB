import KaleidoscopeTunnelBackground from "./KaleidoscopeTunnelBackground.tsx";
import KaleidoscopeTunnelBackgroundGL, {
  type TunnelBackgroundProps,
} from "@/webgl/KaleidoscopeTunnel/KaleidoscopeTunnelBackgroundGL.tsx";

export type { TunnelBackgroundProps };

// Switches between the SVG spiral and its WebGL rewrite while the rewrite
// is being built out milestone by milestone -- flip to "svg" to compare.
// The WebGL side is live from M1 onward, so each milestone's actual
// progress is what's on the real site, not something checked behind a flag.
const ACTIVE: "svg" | "webgl" = "webgl";

export default function TunnelBackground(props: TunnelBackgroundProps) {
  return ACTIVE === "webgl" ? (
    <KaleidoscopeTunnelBackgroundGL {...props} />
  ) : (
    <KaleidoscopeTunnelBackground {...props} />
  );
}
