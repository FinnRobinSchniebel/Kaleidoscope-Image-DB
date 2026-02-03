import { Geist, Geist_Mono } from 'next/font/google';
import React from 'react'


export default function AppLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (

    <div className='flex flex-col bg-foreground min-h-dvh w-full xl:w-6/10 backdrop-blur-[10px] h-full border-white/20 justify-self-center justify-center text-center text-primary font-bold'>
      {children}
      <div className='h-25'></div>
    </div>

  );
}