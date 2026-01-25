import { Carousel, CarouselApi, CarouselContent, CarouselItem } from "@/components/ui/carousel";
import ImageSetViewer from "./ImageSetViewer";
import { DialogContent, DialogDescription, DialogHeader } from "@/components/ui/dialog";
import { DialogTitle } from "@radix-ui/react-dialog";
import { SetData } from "@/components/api/jwt_apis/search-api";
import { JSX, useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Swiper, SwiperClass, SwiperSlide } from 'swiper/react'
import { Images } from "lucide-react";
import { Virtual } from 'swiper/modules';


interface Props {
  imageSets: SetData[]
  setIndex: number
}

export default function VerticalImageSetCarousel({ imageSets, setIndex }: Props) {


  const PRELOADFRONT = 2
  const PRELOADBACK = 3

  const [lockedAxis, setLockedAxis] = useState<"horizontal" | "vertical" | null>(null)
  const startRef = useRef<{ x: number; y: number } | null>(null)

  const hasloaded = useRef(false)


  //lowest ISet Index rendered (Logical)
  const [lowestIndex, setLowestIndex] = useState<number>(Math.max(setIndex - PRELOADFRONT, 0))
  //Highest ISet Index rendered (Logical)
  const [highestIndex, setHighestIndex] = useState<number>(Math.min(setIndex + PRELOADBACK, imageSets.length))
  //ImageSet Index logical
  const LogicalIndex = useRef(setIndex)


  //ImageSet Index, Relative to carousel NOT logic
  const currentIndex = useRef(setIndex - lowestIndex)
  const lastIndex = useRef(setIndex - lowestIndex)
  const pendingShift = useRef(0)

  //api to get info from the imagesets carousel
  const [verticalISetCarouselAPI, setVerticalISetCarouselAPI] = useState<SwiperClass>()

  //ref to determine howmuch movement occured infront of the list
  const frontMovement = useRef(0)


  const onChange = useCallback(() => {
    if (!verticalISetCarouselAPI) {
      return
    }
    const index = currentIndex.current + (verticalISetCarouselAPI.activeIndex - lastIndex.current)
    lastIndex.current = index
    currentIndex.current = index
    console.log(index)

    if (index == 0 && lowestIndex != 0) {



      setLowestIndex((lowestIndex) => {
        const change = Math.min(PRELOADFRONT, lowestIndex)
        console.log(`shift pending: ${change}`)
        pendingShift.current = change
        return lowestIndex - change
      })


    }
    if (index == verticalISetCarouselAPI.slides.length - 1 && highestIndex != Images.length) {
      setHighestIndex((highestIndex) => { return highestIndex + Math.min(PRELOADBACK, imageSets.length - highestIndex) })

      console.log(`heigh: ${highestIndex}, Max: ${imageSets.length} result: ${highestIndex + Math.min(PRELOADBACK, imageSets.length - highestIndex)}`)
    }
  }, [verticalISetCarouselAPI, highestIndex, lowestIndex])

  useLayoutEffect(() => {
    if (!verticalISetCarouselAPI) {
      return
    }

    verticalISetCarouselAPI.on('slideChange', onChange)

    const index = verticalISetCarouselAPI.activeIndex

    return () => {
      verticalISetCarouselAPI.off("slideChange", onChange)
    }
    //verticalISetCarouselAPI.

  }, [verticalISetCarouselAPI, onChange])

  useLayoutEffect(() => {
    if (!verticalISetCarouselAPI) return
    //if (!pendingShift.current) return

    const shift = pendingShift.current
    pendingShift.current = 0

    const target = verticalISetCarouselAPI.activeIndex + shift

    verticalISetCarouselAPI.updateSlides()
    //verticalISetCarouselAPI.slideTo(target, 0, false)
    //console.log(`Triggered reindex ${verticalISetCarouselAPI.activeIndex + shift}`)
  }, [lowestIndex])

  //set location of opened image


  return (
    <div
      onPointerDown={(e) => {
        startRef.current = { x: e.clientX, y: e.clientY }
        setLockedAxis(null)
      }}
      onPointerMove={(e) => {
        if (!startRef.current || lockedAxis) return

        const dx = Math.abs(e.clientX - startRef.current.x)
        const dy = Math.abs(e.clientY - startRef.current.y)

        const THRESHOLD = 6
        if (dx < THRESHOLD && dy < THRESHOLD) return

        setLockedAxis(dx > dy ? "horizontal" : "vertical")
      }}
      onPointerUp={() => {
        startRef.current = null
        setLockedAxis(null)
      }}
      onPointerCancel={() => {
        startRef.current = null
        setLockedAxis(null)
      }}
      className="h-full w-full">

      <Swiper onSwiper={setVerticalISetCarouselAPI}
        modules={[Virtual]}
        direction={'vertical'}
        slidesPerView={1}
        initialSlide={setIndex - lowestIndex}
        centeredSlides={true}
        virtual
        className="h-full ">
        {imageSets.slice(lowestIndex, highestIndex).map((set, index) => (
          <SwiperSlide className="basis-full" virtualIndex={index} key={`imageSet-${set._id}`}>
            <ImageSetViewer set={set} distance={Math.abs(currentIndex.current - index)} DirectionLock={lockedAxis !== "vertical"} />
          </SwiperSlide>
        ))}

      </Swiper>
    </div>
  )


}

//Last carousel index
// const carouselIndexRef = useRef(0)
// const logicalIndexRef = useRef(setIndex)

// useLayoutEffect(() => {
//     if (!verticalISetCarouselAPI) {
//         return
//     }
//     const onSelect = () => {
//         const newCarouselIndex = verticalISetCarouselAPI.selectedScrollSnap()
//         const delta = newCarouselIndex - carouselIndexRef.current

//         if (delta !== 0) {
//             logicalIndexRef.current += delta
//             carouselIndexRef.current = newCarouselIndex

//             setCurrentIndex(logicalIndexRef.current)
//         }
//         console.log(`Cindex: ${carouselIndexRef.current}`)
//         console.log(logicalIndexRef.current)
//     }
//     verticalISetCarouselAPI.on("select", onSelect)
//     console.log(`Cindex: ${carouselIndexRef.current}`)
//     console.log(logicalIndexRef.current)

//     return () => {
//         verticalISetCarouselAPI.off("select", onSelect)
//     }
// }, [verticalISetCarouselAPI])