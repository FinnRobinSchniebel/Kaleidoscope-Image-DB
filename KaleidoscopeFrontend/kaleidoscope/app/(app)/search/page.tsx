
import React from 'react'
import SearchBar from './SearchBar'
import { ScrollArea } from '@radix-ui/react-scroll-area'
import SearchResults from './Results'

type Props = {}

export default function SearchPage({}: Props) {


  return (
    <div className='bg-foreground min-h-dvh w-full xl:w-8/10 backdrop-blur-[10px] h-full border-white/20 justify-self-center justify-center text-center text-primary font-bold'>
      <div className='p-10 text-4xl'>Search</div>
      <SearchBar></SearchBar>      
      <div className=''>
        <SearchResults/>
      </div>
      
    </div>
  )
}