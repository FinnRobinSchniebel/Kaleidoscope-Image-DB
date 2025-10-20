import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";
import Nav from './Nav.tsx'


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

export default function RootLayout({
  children,
}: Readonly<{children: React.ReactNode}>) {
  return (
    <html lang="en"> 
      <body className={`${geistSans.variable} ${geistMono.variable} antialiased`}
      >
         <div className="h-dvh bg-cover w-full" style={{ backgroundImage: `url('/random hexa.png')` }}>
          <div className="font-sans grid grid-rows-[20px_1fr_20px] items-center justify-items-center min-h-screen p-8 pb-20 gap-16 sm:p-20">
            {children}
            <Nav />
          </div>
         </div>       
      </body>
    </html>
  );
}
