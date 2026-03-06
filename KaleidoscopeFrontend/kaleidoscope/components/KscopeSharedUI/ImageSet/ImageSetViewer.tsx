import GetImageSetData, { FullImageSetData } from "@/components/api/GetImageSetData-api";
import { useProtected } from "@/components/api/jwt_apis/ProtectedProvider";
import { SetData } from "@/components/api/jwt_apis/search-api";
import { Carousel, CarouselApi, CarouselContent, CarouselItem, CarouselNext, CarouselPrevious } from "@/components/ui/carousel";
import { JSX, useContext, useEffect, useLayoutEffect, useMemo, useState } from "react";
import ImageSetCarouselImage from "./ImageSetCarouselImage";
import { Item } from "@/components/ui/item";
import { Collapsible } from "@radix-ui/react-collapsible";
import Description from "./Description";
import { Swiper, SwiperClass, SwiperSlide } from "swiper/react";
import { Keyboard, Navigation } from 'swiper/modules';

import "swiper/css";
import 'swiper/css/navigation';
import NavigationHorizontal from "./NavigationHorizontal";
import { HideUIContext } from "./VerticalSetCarousel";
import { SideButtons } from "./SideButtons";
import HitAreaButton from "./HitAreaButton";



interface Props {
  set: SetData
  distance: number
  SetHideUI: () =>void
}


export default function ImageSetViewer({ set, distance, SetHideUI }: Props) {

  //console.log(set)



  const protectedApi = useProtected()

  //contains all image info stored on the server thats relevant
  const [ImageSetInfo, SetImageSetInfo] = useState<FullImageSetData>()

  //api to get info from the image carousel
  const [api, setApi] = useState<SwiperClass>()

  const [CurrentIndex, setCurrentIndex] = useState(0)

  const [isDescriptionOpen, setIsDescriptionOpen] = useState<boolean>(false)

  useLayoutEffect(() => {

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
    setCurrentIndex(api.activeIndex)

    api.on("activeIndexChange", () => {
      setCurrentIndex(api.activeIndex)
    })

  }, [api])

  // useEffect(() => {
  //   console.log(`ID: ${set._id} distance: ${distance} currentIndex: ${CurrentIndex}`)
  // }, [distance])


  return (
    <div className="flex flex-col h-full min-h-0">
      <div className="flex-1 relative min-h-0">
        <HitAreaButton active={distance == 0 && !isDescriptionOpen} onHit={SetHideUI} debugClassName="bg-amber-50/50" className={`absolute flex justify-self-center w-3/5 h-4/5 z-2 pointer-events-none`} >
        </HitAreaButton>

        <Swiper
          direction="horizontal"
          onSwiper={setApi}
          modules={[Keyboard]}
          keyboard={{
            enabled: true,
            onlyInViewport: true
          }}

          slidesPerView={1}
          centeredSlides={true}
          className="relative image-set-swiper text-primary h-full w-full"
        >
          <NavigationHorizontal activeSet={distance == 0} api={api} index={CurrentIndex} Count={set.activeImageCount} />
          {Array.from({ length: set.activeImageCount }, (_, index) => (
            <SwiperSlide className="h-full justify-items-center ">
              <ImageSetCarouselImage
                key={`${set._id}-${index}`}
                SetID={set._id}
                index={index}
                distance={distance}
                currentIndex={CurrentIndex}
                keepLoadedOverride={false}
              />
            </SwiperSlide>
          ))}
        </Swiper>
        <SideButtons active={distance == 0} Disabled={false} />
        <Description info={ImageSetInfo} WhenOpenCallback={(e)=>{setIsDescriptionOpen(e)}}/>
      </div>
      <div className="flex justify-center items-center mb-3 mt-1">
        <Item className=" bg-background/40 backdrop-blur-sm border-2 border-background/60 min-w-20 max-w-40 justify-center overflow-hidden" variant={"outline"}>
          {CurrentIndex + 1}/{set.activeImageCount}
        </Item>
      </div>

      {/* image count */}
      {/* image slider */}
      {/* tags */}
      {/* discription */}

    </div>

  )

}