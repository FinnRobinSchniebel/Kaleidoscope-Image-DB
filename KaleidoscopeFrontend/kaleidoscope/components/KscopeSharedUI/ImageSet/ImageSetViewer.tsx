import GetImageSetData, { FullImageSetData } from "@/components/api/GetImageSetData-api";
import { useProtected } from "@/components/api/jwt_apis/ProtectedProvider";
import { SetData } from "@/components/api/jwt_apis/search-api";
import { Carousel, CarouselApi, CarouselContent, CarouselItem, CarouselNext, CarouselPrevious } from "@/components/ui/carousel";
import { JSX, useEffect, useLayoutEffect, useState } from "react";
import ImageSetCarouselImage from "./ImageSetCarouselImage";
import { Item } from "@/components/ui/item";
import { Collapsible } from "@radix-ui/react-collapsible";
import Description from "./Description";





interface Props {
  set: SetData
  current: boolean
  DirectionLock: boolean
}


export default function ImageSetViewer({ set, current, DirectionLock }: Props) {

  //console.log(set)

  const protectedApi = useProtected()

  //contains all image info stored on the server thats relevant
  const [ImageSetInfo, SetImageSetInfo] = useState<FullImageSetData>()

  //api to get info from the image carousel
  const [api, setApi] = useState<CarouselApi>()

  //contains the image elements in the carousel as finished react elements
  const [CarouselImages, SetCarouselImages] = useState<JSX.Element[]>([])


  //TODO: get index from url
  const [CurrentIndex, setCurrentIndex] = useState(0)

  useLayoutEffect(() => {
    const elements = Array.from({ length: set.activeImageCount }, (_, index) => (
      <ImageSetCarouselImage
        key={`${set._id}-${index}`} // unique key per element
        SetID={set._id}
        Index={index}
        Load={current ? Math.abs(CurrentIndex - index) <= 5 : CurrentIndex == index}
      />
    ))
    SetCarouselImages(elements)

    console.log(`set: (id: ${set._id}, count: ${set.activeImageCount}, tags: ${set.tags}), current index: ${CurrentIndex}, Carousel items: ${CarouselImages.length}`)


    //get the image data 
    const getData = async () => {
      const imageInfo = await GetImageSetData({ id: set._id, protectedApi: protectedApi })
      SetImageSetInfo(imageInfo)
    }
    getData()


  }, [set])

  useLayoutEffect(() => {
    if (!api) {
      return
    }
    setCurrentIndex(api.selectedScrollSnap() + 1)

    api.on("select", () => {
      setCurrentIndex(api.selectedScrollSnap() + 1)
    })

  }, [api])


  return (
    <div className="flex flex-col h-full min-h-0">
      <div className="flex-1 relative min-h-0">
        <Carousel setApi={setApi} opts={{ align: "center", duration: 0, watchDrag: DirectionLock }} className="flex justify-center text-primary h-full w-full ">
          <CarouselContent className=" h-full w-full smin-h-0">
            {CarouselImages}
          </CarouselContent>
          <CarouselPrevious />
          <CarouselNext />
        </Carousel>
        <Description info={ImageSetInfo} />
      </div>
      <div className="flex justify-center items-center mb-3 mt-1">
        <Item className=" bg-background/40 backdrop-blur-sm border-2 border-background/60 min-w-20 max-w-40 justify-center overflow-hidden" variant={"outline"}>
          {CurrentIndex}/{set.activeImageCount}
        </Item>
      </div>

      {/* image count */}
      {/* image slider */}
      {/* tags */}
      {/* discription */}

    </div>

  )

}