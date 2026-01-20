'use client'

import { imageAPI, imageRequest } from "@/components/api/image-api";
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { useProtected } from "@/components/api/jwt_apis/ProtectedProvider";
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
  index: number
  OpenImageSet : (i: number)=> void 
}


export default function ImageCard(cardInfo: Card) {

  const [image, setImage] = useState<string>("")

  const protectedApi = useProtected()

  var request: imageRequest = useMemo(() => ({
    protectedApiRef: protectedApi,
    ID: cardInfo.id,
    Index: 0,
    Lowres: true
  }), [cardInfo.id, protectedApi])

  useEffect(() => {

    let cancelled = false
    const t = async () => {

      const url = await imageAPI(request)
      if (cancelled) {
        URL.revokeObjectURL(url)
        return
      }

      setImage(prev => {
        if (prev) URL.revokeObjectURL(prev)
        return url
      })
    }
    t()

    return () => {
      cancelled = true
    }

  }, [cardInfo.id, cardInfo.Tags, request])
  // const data = use(imageAPI(request))


  if (image != "") {
    return (
      <li key={"li-card-" + cardInfo.id}>
        <button onClick={() => {cardInfo.OpenImageSet(cardInfo.index)}} key={"card-button-" + cardInfo.id} className="relative size-60 md:size-80 lg:size-80 2xl:size-90 md:m-[1px] 2xl:m-[4px]">
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