import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";

import Nav from '../../components/KscopeSharedUI/Nav.tsx'
import KaleidoscopeTunnelBackground from "@/components/KscopeSharedUI/KaleidoscopeTunnel/KaleidoscopeTunnelBackground.tsx";


const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "Auth Kaleidoscope",
  description: "Login to your DB",
};

export default function AuthLayout({ children, }: Readonly<{ children: React.ReactNode }>) {

  return (
    <html lang="en">
      <body className={`${geistSans.variable} ${geistMono.variable} antialiased h-dvh `}
      >
        <div className="flex items-center justify-center bg-fixed bg-cover h-full object-cover">
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
      </body>
    </html>
  );
}
