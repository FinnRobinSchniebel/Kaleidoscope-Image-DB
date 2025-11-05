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
  title: "Auth Kaleidoscope",
  description: "Login to your DB",
};

export default function AuthLayout({children,}: Readonly<{ children: React.ReactNode }>) {

  return (
    <html lang="en">
      <body className={`${geistSans.variable} ${geistMono.variable} antialiased h-dvh `}
      >
        <div className="flex items-center justify-center bg-fixed bg-cover h-full object-cover bg-[url('/random%20hexa.png')]">
          {children}
          
        </div>
      </body>
    </html>
  );
}
