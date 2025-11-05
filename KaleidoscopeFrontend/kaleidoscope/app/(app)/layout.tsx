import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./../globals.css";
import Nav from '../Nav.tsx'


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
      <body className={`${geistSans.variable} ${geistMono.variable} antialiased min-h-dvh `}
      >
        {/* <div className="fixed h-full bg-cover w-full bg-[url('/random%20hexa.png')]" /> */}
        <div className="justify-items-center bg-fixed bg-cover h-full object-cover bg-[url('/random%20hexa.png')]">
          {children}
          <Nav />
        </div>
      </body>
    </html>
  );
}
