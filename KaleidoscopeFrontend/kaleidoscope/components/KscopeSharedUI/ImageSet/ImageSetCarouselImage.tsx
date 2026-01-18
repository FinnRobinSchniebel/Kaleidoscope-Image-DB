import { LoadingImageCard } from "@/app/(app)/search/ImageCards"
import { imageAPI, imageRequest } from "@/components/api/image-api"
import { useProtected } from "@/components/api/jwt_apis/ProtectedProvider"
import { CarouselItem } from "@/components/ui/carousel"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"
import Image from "next/image"
import { useEffect, useMemo, useState } from "react"


interface Props {
  SetID: string
  index: number
  distance: number
  currentIndex: number
  keepLoadedOverride : boolean
}


export default function ImageSetCarouselImage({ SetID, index: index, distance, currentIndex }: Props) {


  const [image, setImage] = useState<string | null>(null)

  const load = (distance == 0 ? Math.abs(currentIndex - index) <= 5 : distance > 5 ? false : currentIndex == index) || currentIndex

  const protectedApi = useProtected()

  var request: imageRequest = useMemo(() => ({
    protectedApiRef: protectedApi,
    ID: SetID,
    Index: index,
    Lowres: true
  }), [SetID, protectedApi])

  useEffect(() => {

    //console.log("imageLoad triggered")
    let cancelled = false
    if (!load) {
      //console.log("revoked image")
      setImage(prev => {
        if (prev) URL.revokeObjectURL(prev)
        return null
      })
      return
    }

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

  }, [SetID, index, load, request])

  if (image) {
    return (
      <CarouselItem className="h-full w-full flex justify-center">
        <img src={image} alt="'/random%20hexa.png'" className=" h-full object-contain" />
      </CarouselItem>
    )
  }

  return (
    <CarouselItem className="h-full w-full flex justify-center">
      <Skeleton className={cn("h-full w-full")}></Skeleton>
    </CarouselItem>
  )



}