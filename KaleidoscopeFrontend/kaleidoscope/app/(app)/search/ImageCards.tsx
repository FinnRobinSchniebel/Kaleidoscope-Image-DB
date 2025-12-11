'use client'

import { imageAPI, imageRequest } from "@/components/api/jwt_apis/image-api";
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { searchAPI } from "@/components/api/jwt_apis/search-api";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { Suspense, use, useEffect } from "react";

interface Card {
  id: string;
  Tags?: string[];
  protAPI: protectedAPI

}


export default function ImageCard(cardInfo: Card) {

  var request : imageRequest =  {
    protectedApiRef: cardInfo.protAPI,
    ID: cardInfo.id,
    Index: 0,
    Lowres: true
  }


  const data = use(imageAPI(request))

  return (
    <li key={"li-card-" + cardInfo.id}>
      <button key={"card-button-" + cardInfo.id} className="size-60 md:size:80 lg:size-80 2xl:size-90 md:m-[1px] 2xl:m-[4px] bg-[url('/random%20hexa.png')]">
        <Skeleton key={"card-temp-" + cardInfo.id} className={cn("size-60 md:size:80 lg:size-80 2xl:size-90 md:m-[1px] 2xl:m-[4px]")}></Skeleton>
      </button>
    </li>
  )
}

export function LoadingImageCard() {
  return (
    <Skeleton className={cn("size-60 md:size:80 lg:size-80 2xl:size-90 md:m-[1px] 2xl:m-[4px]")}></Skeleton>
  )

}


//size-60 md:size:80 2xl:size-90 lg:size-70  2xl:m-5
//<Suspense fallback={<LoadingImageCard/>}>

function wait(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms));
}