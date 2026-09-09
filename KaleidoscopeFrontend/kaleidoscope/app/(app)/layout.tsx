import type { Metadata } from "next";

import Nav from '../../components/KscopeSharedUI/Nav.tsx'
import { Toaster } from "@/components/ui/sonner.tsx";
import KaleidoscopeTunnelBackground from '../../components/KscopeSharedUI/KaleidoscopeTunnel/KaleidoscopeTunnelBackground.tsx'

export const metadata: Metadata = {
  title: "Kaleidoscope",
  description: "An Image DB viewing frontend",
};

export default function AppLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <div className="relative flex-grow justify-items-center h-full">
      <KaleidoscopeTunnelBackground turns={2.5}
        baseWidth={1.5}
        startDepth={0}
        depth={9000}
        slantWeight={0.0}
        tipFocusX={0.95}
        tipFocusY={0.2}
        baseFocusX={-1}
        baseFocusY={1.9}
        rotationPeriod={1200} />
      {children}
      <Nav />
      <Toaster />
    </div>
  );
}
