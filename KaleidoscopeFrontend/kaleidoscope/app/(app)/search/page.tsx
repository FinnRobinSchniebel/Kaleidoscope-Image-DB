
import React from 'react'
import SearchBar from './SearchBar'
import { ScrollArea } from '@radix-ui/react-scroll-area'
import SearchResults from './Results'
import Search from './Search'
import { protectedAPI } from '@/components/api/jwt_apis/protected-api-client'
import { ReadToken } from '@/components/api/get_variables_server'

type Props = {}

export default async function SearchPage({}: Props) {
  
  //gets the token as it is when the page is rendered 
  const token = await ReadToken()

  return (
    <div className='bg-foreground min-h-dvh w-full xl:w-8/10 backdrop-blur-[10px] h-full border-white/20 justify-self-center justify-center text-center text-primary font-bold'>
      <div className='p-10 text-4xl'>Search</div>
      <Search token={token} />
    </div>
  )
}