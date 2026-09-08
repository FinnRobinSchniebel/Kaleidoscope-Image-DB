import type { Metadata } from "next";
import KaleidoscopeTunnelBackground from "@/components/KscopeSharedUI/KaleidoscopeTunnel/KaleidoscopeTunnelBackground.tsx";

export const metadata: Metadata = {
  title: "Auth Kaleidoscope",
  description: "Login to your DB",
};

export default function AuthLayout({ children, }: Readonly<{ children: React.ReactNode }>) {

  return (
    <div className="flex items-center justify-center bg-fixed bg-cover flex-grow object-cover">
      <KaleidoscopeTunnelBackground turns={2.5}
        baseWidth={2}
        startDepth={0}
        depth={9000}
        slantWeight={0.0}
        tipFocusX={0.5}
        tipFocusY={0.5}
        baseFocusX={0.5}
        baseFocusY={0.5}
        rotationPeriod={1200} />
      {children}

    </div>
  );
}
