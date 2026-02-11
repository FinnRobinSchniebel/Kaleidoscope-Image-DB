import { ReadToken } from '@/components/api/get_variables_server';
import { ProtectedProvider } from '@/components/api/jwt_apis/ProtectedProvider';
import { Geist, Geist_Mono } from 'next/font/google';
import React, { useContext } from 'react'


export default async function AppLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {

  const token = await ReadToken()


  return (

    <div className='flex flex-col bg-foreground min-h-dvh w-full xl:w-6/10 backdrop-blur-[10px] h-full border-white/20 justify-self-center justify-center text-center text-primary font-bold'>
      <ProtectedProvider token={token} >
        {children}
      </ProtectedProvider>
      <div className='h-25'></div>
    </div>

  );
}