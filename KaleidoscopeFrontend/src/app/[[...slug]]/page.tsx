import '../../index.css'
import React from 'react'
import dynamic from 'next/dynamic'
import ClientOnly from "./client"

 
export function generateStaticParams() {
  return [{ slug: [''] }]
}
 
export default function Page() {
  return <ClientOnly />
}