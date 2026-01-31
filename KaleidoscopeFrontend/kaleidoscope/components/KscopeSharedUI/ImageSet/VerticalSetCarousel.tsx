import ImageSetViewer from "./ImageSetViewer";

import { SetData } from "@/components/api/jwt_apis/search-api";
import { createContext, PointerEvent, useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Swiper, SwiperClass, SwiperSlide } from 'swiper/react'

import { Virtual } from 'swiper/modules';
import 'swiper/css';
import 'swiper/css/navigation';
import HitAreaButton from "./HitAreaButton";


interface Props {
  imageSets: SetData[]
  setIndex: number
}

type HitTarget = {
  id: string
  rect: () => DOMRect | null
  onHit: () => void
}

type HitTestContextType = {
  debug: boolean
  register: (t: HitTarget) => void
  unregister: (id: string) => void
}

export const HitTestContext = createContext<HitTestContextType | null>(null)


export default function VerticalImageSetCarousel({ imageSets, setIndex }: Props) {


  const [debug, setDebug] = useState(true)


  const PRELOADFRONT = 2
  const PRELOADBACK = 3

  const PointerDownLoc = useRef<{ x: number; y: number } | null>(null)


  const pointerUpLoc = useRef<{ x: number; y: number } | null>(null)

  const [HideOverlayes, setHideOverlayes] = useState(false)

  const [currentIndex, setCurrentIndex] = useState(setIndex)

  //api to get info from the imagesets carousel
  const verticalISetCarouselAPI = useRef<SwiperClass>(undefined)

  const hitTargets = useRef<HitTarget[]>([])

  //Keeps track of the current index of the carousel 
  const onChange = useCallback(() => {
    if (!verticalISetCarouselAPI.current) {
      return
    }
    const index = verticalISetCarouselAPI.current.activeIndex

    setCurrentIndex(index)
    console.log(`index: ${index}`)

  }, [verticalISetCarouselAPI])

  //add listiners to carousel and remove the event if the carousel is removed
  useLayoutEffect(() => {
    if (!verticalISetCarouselAPI.current) {
      return
    }
    verticalISetCarouselAPI.current.on('activeIndexChange', onChange)

    return () => {
      if (!verticalISetCarouselAPI.current) {
        return
      }
      verticalISetCarouselAPI.current.off("activeIndexChange", onChange)
    }


  }, [verticalISetCarouselAPI, onChange])

  //used for managing all button overlays on the carousel. This is the only way I found to make the carousel interactive with buttons on top of it.
  const register = useCallback((t: HitTarget) => {
    hitTargets.current.push(t)
  }, [])

  const unregister = useCallback((id: string) => {
    hitTargets.current = hitTargets.current.filter(t => t.id !== id)
  }, [])



  const handleTap = useCallback((e: MouseEvent | TouchEvent | globalThis.PointerEvent) => {
    // verticalISetCarouselAPI.current?.emit("")
    //setHideOverlayes((overlay) => {
    console.log("tap");

    var hitLoc: { x: number; y: number } = ({x:0 , y:0})

    if (e instanceof MouseEvent) {
      hitLoc = { x: e.clientX, y: e.clientY }
    }
    else if (e instanceof TouchEvent) {
      console.log(e.changedTouches.length)
      hitLoc = { x: e.changedTouches[0].clientX, y: e.changedTouches[0].clientY }
    }
    else {
      console.log("unknown pointEvent made")
    }

    hitTargets.current.forEach(element => {
      const rect = element.rect()
      if (!rect) return
      if (
        hitLoc.x >= rect.left &&
        hitLoc.x <= rect.right &&
        hitLoc.y >= rect.top &&
        hitLoc.y <= rect.bottom
      ) {
        element.onHit()
      }
    });


  }, []);

  const buttonRef = useRef<HTMLDivElement | null>(null)
  const ButtonSize = useRef<{ x: number; y: number } | null>(null)
  const ButtonLocation = useRef<{ x: number; y: number } | null>(null)

  useEffect(() => {
    const loc = ButtonLocation.current
    const size = ButtonSize.current
    if (!loc || !size || !pointerUpLoc.current) return

    if (pointerUpLoc.current.x >= loc.x &&
      pointerUpLoc.current.x <= loc.x + size.x &&
      pointerUpLoc.current.y >= loc.y &&
      pointerUpLoc.current.y <= loc.y + size.y) {
      console.log('inside')
    }
  }, [pointerUpLoc.current])

  useEffect(() => {
    if (!buttonRef.current) return
    const rect = buttonRef.current.getBoundingClientRect()
    ButtonSize.current = {
      x: rect.width,
      y: rect.height,
    }

    ButtonLocation.current = {
      x: rect.left,
      y: rect.top,
    }
  }, [buttonRef])

  
  return (
    <HitTestContext.Provider value={{debug, register, unregister }}>
      <div
        className="h-full w-full ">
        <HitAreaButton onHit={()=>{console.log(`in area center`)}} debugClassName="bg-amber-50/50" className={`absolute flex justify-self-center w-3/5 h-4/5 z-2 bg-accent pointer-events-none `} >
        </HitAreaButton>


        <Swiper
          onSwiper={(swiper) => { verticalISetCarouselAPI.current = swiper }}
          modules={[Virtual]}
          direction={'vertical'}
          slidesPerView={1}
          initialSlide={setIndex}
          // centeredSlides={true}
          watchSlidesProgress
          longSwipes={false}
          spaceBetween={1}
          virtual={{ addSlidesAfter: 4, addSlidesBefore: 2, slides: imageSets }}
          className="h-full "
          onTap={(_, e) => handleTap(e)}
        >

          {imageSets.map((set, index) => (
            <SwiperSlide className="h-full" virtualIndex={index} key={`imageSet-${set._id}`}>
              <ImageSetViewer set={set} distance={Math.abs(currentIndex - index)} />
            </SwiperSlide>
          ))}

        </Swiper>
      </div>
      
    </HitTestContext.Provider>
  )


}


// useEffect(() => {
  //   const loc = ButtonLocation.current
  //   const size = ButtonSize.current
  //   if (!loc || !size || !pointerUpLoc.current) return 



  //   if (pointerUpLoc.current.x >= loc.x &&
  //     pointerUpLoc.current.x <= loc.x + size.x &&
  //     pointerUpLoc.current.y >= loc.y &&
  //     pointerUpLoc.current.y <= loc.y + size.y) {
  //     console.log('inside')
  //   }

  //   PointerDownLoc.current = null
  // }, [pointerUpLoc.current])

  // function that on change of pointerUpLoc = useRef<{ x: number; y: number } | null>(null) change will check if the up and down are both in the area of the element

// if (index == 0 && lowestIndex != 0) {



//   setLowestIndex((lowestIndex) => {
//     const change = Math.min(PRELOADFRONT, lowestIndex)
//     console.log(`shift pending: ${change}`)
//     pendingShift.current = change
//     return lowestIndex - change
//   })


// }
// if (index == verticalISetCarouselAPI.slides.length - 1 && highestIndex != Images.length) {
//   setHighestIndex((highestIndex) => { return highestIndex + Math.min(PRELOADBACK, imageSets.length - highestIndex) })

//   console.log(`heigh: ${highestIndex}, Max: ${imageSets.length} result: ${highestIndex + Math.min(PRELOADBACK, imageSets.length - highestIndex)}`)
// }


// useLayoutEffect(() => {
//   if (!verticalISetCarouselAPI) return
//   //if (!pendingShift.current) return

//   const shift = pendingShift.current
//   pendingShift.current = 0

//   const target = verticalISetCarouselAPI.activeIndex + shift

//   verticalISetCarouselAPI.updateSlides()
//   //verticalISetCarouselAPI.slideTo(target, 0, false)
//   //console.log(`Triggered reindex ${verticalISetCarouselAPI.activeIndex + shift}`)
// }, [lowestIndex])




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