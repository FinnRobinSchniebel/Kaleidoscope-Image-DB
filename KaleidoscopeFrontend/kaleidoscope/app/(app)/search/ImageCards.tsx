'use client'

import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { Suspense } from "react";

interface Card {
  id?: string;
  Tags?: string[];
  className? : string | undefined,
}


export default async function ImageCard(params: Card) {
  
  return (
  <div>
    <button className="w-25 h-25 bg-[url(/random hexa.png)]">
      <LoadCard />
    </button>
  </div>
  )
}

export function LoadingImageCard({id, Tags, className} : Card){
  return (
    <Skeleton className={cn("size-60 md:size:80 lg:size-80 2xl:size-90 md:m-[1px] 2xl:m-[4px]", className)}></Skeleton>
  )
  
}

async function LoadCard(){
  await new Promise((resolve) =>setTimeout(resolve, 4000))
  
  return (
    <></>
  )

}
//size-60 md:size:80 2xl:size-90 lg:size-70  2xl:m-5
//<Suspense fallback={<LoadingImageCard/>}>