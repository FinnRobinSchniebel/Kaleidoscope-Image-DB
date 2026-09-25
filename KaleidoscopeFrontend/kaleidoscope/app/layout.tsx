import type { Metadata, Viewport } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";

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

// No theme provider is wired up anywhere in this app (next-themes is a
// dependency but unused for now) -- this is a light-only page. Without an
// explicit color-scheme, a browser on a dark-mode OS paints its very first
// frame with a dark/grey placeholder before this page's actual (light,
// blue-grey) content ever arrives, which shows as a brief flash/flicker on
// initial load, independent of anything in the tunnel background itself.
export const viewport: Viewport = {
  colorScheme: "light",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body className={`${geistSans.variable} ${geistMono.variable} antialiased min-h-dvh flex flex-col bg-[#5a8697]`}
      >
        {children}
      </body>
    </html>
  );
}
