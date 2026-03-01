'use client'

import { imageAPI, imageRequest, ImageRequestToString, thumbNailAPI } from "@/components/api/image-api";
import { imageCache } from "@/components/api/ImageCaching";
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { useProtected } from "@/components/api/jwt_apis/ProtectedProvider";
import { searchAPI } from "@/components/api/jwt_apis/search-api";
import ImageSetViewer from "@/components/KscopeSharedUI/ImageSet/ImageSetViewer";
import VerticalImageSetCarousel from "@/components/KscopeSharedUI/ImageSet/VerticalSetCarousel";
import { DialogTrigger } from "@/components/ui/dialog";
import { Item } from "@/components/ui/item";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

import Image from "next/image";
import { Suspense, use, useEffect, useMemo, useState } from "react";

interface Card {
  id: string;
  Tags?: string[];
  index: number
  imageCount: number
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

      const requestName = `${request.ID}-thumb`

      const url = await imageCache.get(requestName, async ()=>{const {blob, err} = await thumbNailAPI(request); return {blob: blob, err: err}} , "")

      

      setImage(url)
    }
    t()

    return () => {
      cancelled = true
    }

  }, [cardInfo.id, cardInfo.Tags, request])
  // const data = use(imageAPI(request))


  if (image != "") {
    return (
      <li key={"li-card-" + cardInfo.id} >
        <button onClick={() => {cardInfo.OpenImageSet(cardInfo.index)}} key={"card-button-" + cardInfo.id} className="aspect-square relative object-cover w-full h-full rounded-md overflow-hidden">
          <Image src={image} alt="'/random%20hexa.png'" className="object-cover pointer-events-none" fill ></Image>
          <div className="absolute bg-background/20 backdrop-blur-sm border-1 border-background/60 max-w-40 overflow-hidden right-2 top-2 p-1.5 rounded-tl-sm rounded-br-sm rounded-bl-xl rounded-tr-xl"> {cardInfo.imageCount} </div>
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