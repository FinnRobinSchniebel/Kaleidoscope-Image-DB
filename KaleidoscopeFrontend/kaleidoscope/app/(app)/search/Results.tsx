'use client'

import { ScrollArea } from "@/components/ui/scroll-area"
import ImageCard, { LoadingImageCard } from "./ImageCards"
import { Fragment } from "react";
import { Separator } from "@radix-ui/react-separator";


export default function SearchResults() {

  const elementsArray = Array.from({ length: 30 }, (_, index) => (
    <li key={'li-' + index}>
      <LoadingImageCard key={index} className='' />
    </li>

  ));

  const tags = Array.from({ length: 50 }).map(
    (_, i, a) => `v1.2.0-beta.${a.length - i}`
  )

  return (

    <ul className="w-full flex flex-wrap pb-[15%] lg:pb-[6.5%] xl:pb-[4%] justify-center">
      
      {elementsArray}

    </ul>
  )
}

//grid-cols-2 md:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-5