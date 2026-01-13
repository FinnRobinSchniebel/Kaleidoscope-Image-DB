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
  Index: number
  Load: boolean
}


export default function ImageSetCarouselImage({ SetID, Index, Load }: Props) {


  const [image, setImage] = useState<string | null>(null)

  const protectedApi = useProtected()

  var request: imageRequest = useMemo(() => ({
    protectedApiRef: protectedApi,
    ID: SetID,
    Index: Index,
    Lowres: true
  }), [SetID, protectedApi])

  useEffect(() => {
    let cancelled = false
    if (!Load) {
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

  }, [SetID, Index, Load, request])

  // return(
  //   <CarouselItem>
  //     <Skeleton className={cn("h-[2400px] w-[600px]")}></Skeleton>
  //   </CarouselItem>
  // )
  if (image) {
    return (
      <CarouselItem className="h-full w-full flex justify-center">
        <img src={image} alt="'/random%20hexa.png'" className=" h-full object-contain"/>
      </CarouselItem>
    )
  }
  else {
    <CarouselItem>
      <Skeleton className={cn("h-20 w-40")}></Skeleton>
    </CarouselItem>

  }

}