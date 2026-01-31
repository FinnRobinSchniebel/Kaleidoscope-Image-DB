import { LoadingImageCard } from "@/app/(app)/search/ImageCards"
import { imageAPI, imageRequest } from "@/components/api/image-api"
import { useProtected } from "@/components/api/jwt_apis/ProtectedProvider"
import { CarouselItem } from "@/components/ui/carousel"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"
import Image from "next/image"
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"


interface Props {
  SetID: string
  index: number
  distance: number
  currentIndex: number
  keepLoadedOverride: boolean
}


export default function ImageSetCarouselImage({ SetID, index: index, distance, currentIndex }: Props) {


  const [image, setImage] = useState<string | null>(null)

  

  let load = useRef(false)// (distance == 0 ? Math.abs(currentIndex - index) <= 5 : distance > 5 ? false : currentIndex == index) || currentIndex

  const protectedApi = useProtected()

  var request: imageRequest = useMemo(() => ({
    protectedApiRef: protectedApi,
    ID: SetID,
    Index: index,
    Lowres: true
  }), [SetID, protectedApi])

  const shouldLoad = () => {

    if(load && distance < 10){
      return true
    }
    if(distance > 1 && index == currentIndex){
      return true
    }
    if(distance == 1){
      return true
    }

    return false
  }

  useLayoutEffect(() => {

    //console.log("imageLoad triggered")
    let cancelled = false
    load.current = shouldLoad()
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

      <img src={image} alt="'/random%20hexa.png'" className="h-full object-contain justify-center items-center" />

    )
  }

  return (
    <div className="h-full w-full bg-foreground/80">
      <Skeleton className={cn("h-full w-full")}></Skeleton>
    </div>
  )



}