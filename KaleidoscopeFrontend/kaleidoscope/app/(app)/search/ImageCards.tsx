'use client'

import { imageAPI, imageRequest } from "@/components/api/jwt_apis/image-api";
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { searchAPI } from "@/components/api/jwt_apis/search-api";
import ImageSetViewer from "@/components/KscopeSharedUI/ImageSet/ImageSetViewer";
import VerticalImageSetCarousel from "@/components/KscopeSharedUI/ImageSet/VerticalSetCarousel";
import { DialogTrigger } from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import Image from "next/image";
import { Suspense, use, useEffect, useMemo, useState } from "react";

interface Card {
  id: string;
  Tags?: string[];
  protAPI: protectedAPI
  OpenImageSet : (i: number)=> void 
}


export default function ImageCard(cardInfo: Card) {

  const [image, setImage] = useState<string>("")

  var request: imageRequest = useMemo(() => ({
    protectedApiRef: cardInfo.protAPI,
    ID: cardInfo.id,
    Index: 0,
    Lowres: true
  }), [cardInfo.id, cardInfo.protAPI])

  useEffect(() => {
    const t = async () => {
      setImage(await imageAPI(request))
    }
    t()
  }, [cardInfo.id, cardInfo.Tags])
  // const data = use(imageAPI(request))

  const f = () =>{
    console.log("test")
  }

  if (image != "") {
    return (
      <li key={"li-card-" + cardInfo.id}>
        <button onClick={() => {cardInfo.OpenImageSet(1)}} key={"card-button-" + cardInfo.id} className="relative size-60 md:size-80 lg:size-80 2xl:size-90 md:m-[1px] 2xl:m-[4px]">
          <Image src={image} alt="'/random%20hexa.png'" className="object-cover  pointer-events-none" fill ></Image>
        </button>



      </li>
    )
  }
  return (<LoadingImageCard></LoadingImageCard>)

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