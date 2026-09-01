import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";

import Nav from '../../components/KscopeSharedUI/Nav.tsx'
import { Toaster } from "@/components/ui/sonner.tsx";
import KaleidoscopeTunnelBackground from '../../components/KscopeSharedUI/KaleidoscopeTunnel/KaleidoscopeTunnelBackground.tsx'


const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "Kaleidoscope",
  description: "An Image DB viewing frontend",
};

export default function AppLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body className={`${geistSans.variable} ${geistMono.variable} antialiased min-h-dvh flex flex-col`}
      >
        <div className="relative flex-grow justify-items-center h-full">
          <KaleidoscopeTunnelBackground />
          {children}
          <Nav />
          <Toaster/>
        </div>
      </body>
    </html>
  );
}
