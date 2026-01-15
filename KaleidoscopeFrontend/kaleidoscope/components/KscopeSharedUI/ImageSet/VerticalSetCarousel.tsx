import { Carousel, CarouselApi, CarouselContent, CarouselItem } from "@/components/ui/carousel";
import ImageSetViewer from "./ImageSetViewer";
import { DialogContent, DialogDescription, DialogHeader } from "@/components/ui/dialog";
import { DialogTitle } from "@radix-ui/react-dialog";
import { SetData } from "@/components/api/jwt_apis/search-api";
import { JSX, useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";


interface Props {
  imageSets: SetData[]
  setIndex: number
}

export default function VerticalImageSetCarousel({ imageSets, setIndex }: Props) {


  const [lockedAxis, setLockedAxis] = useState<"horizontal" | "vertical" | null>(null)
  const startRef = useRef<{ x: number; y: number } | null>(null)

  //ImageSet Index
  const [currentIndex, setCurrentIndex] = useState<number>(setIndex)


  //api to get info from the imagesets carousel
  const [verticalISetCarouselAPI, setVerticalISetCarouselAPI] = useState<CarouselApi>()

  const [loadingMore, setLoadingMore] = useState(false)
  const listenForScrollRef = useRef(true)
  const hasMoreToLoadRef = useRef(true)
  const scrollListenerRef = useRef(() => undefined)
  const [hasMoreToLoad, setHasMoreToLoad] = useState(true)

  useLayoutEffect(() => {
    if (!verticalISetCarouselAPI) {
      return
    }
    const onSelect = () => {
      const newCarouselIndex = verticalISetCarouselAPI.selectedScrollSnap()
      setCurrentIndex(newCarouselIndex)
      console.log(newCarouselIndex)
    }
    verticalISetCarouselAPI.on("select", onSelect)

    return () => {
      verticalISetCarouselAPI.off("select", onSelect)
    }
  }, [verticalISetCarouselAPI])

  const watchslideLogic = (verticalISetCarouselAPI: CarouselApi) => {
    if (!verticalISetCarouselAPI) return
    const reloadEmbla = () => {
      if (!verticalISetCarouselAPI) return

      const oldEngine = verticalISetCarouselAPI.internalEngine()

      verticalISetCarouselAPI.reInit()
      const newEngine = verticalISetCarouselAPI.internalEngine()
      const copyEngineModules = [
        'scrollBody',
        'location',
        'offsetLocation',
        'previousLocation',
        'target'
      ] as const
      copyEngineModules.forEach((engineModule) => {
        Object.assign(newEngine[engineModule], oldEngine[engineModule])
      })

      newEngine.translate.to(oldEngine.location.get())
      const { index } = newEngine.scrollTarget.byDistance(0, false)
      newEngine.index.set(index)
      newEngine.animation.start()

      setLoadingMore(false)
      listenForScrollRef.current = true
    }

    const reloadAfterPointerUp = () => {
      if (!verticalISetCarouselAPI) return
      verticalISetCarouselAPI.off('pointerUp', reloadAfterPointerUp)
      reloadEmbla()
    }

    const engine = verticalISetCarouselAPI.internalEngine()

    if (hasMoreToLoadRef.current && engine.dragHandler.pointerDown()) {
      if (!verticalISetCarouselAPI) return
      const boundsActive = engine.limit.reachedMax(engine.target.get())
      engine.scrollBounds.toggleActive(boundsActive)
      verticalISetCarouselAPI.on('pointerUp', reloadAfterPointerUp)
    } else {
      reloadEmbla()
    }
  }

  const onScroll = useCallback((verticalISetCarouselAPI: CarouselApi) => {
    if (!listenForScrollRef.current) return undefined

    setLoadingMore((loadingMore) => {
       if (!verticalISetCarouselAPI) return false
      const lastSlide = verticalISetCarouselAPI.slideNodes().length - 1
      const lastSlideInView = verticalISetCarouselAPI.slidesInView().includes(lastSlide)
      const loadMore = !loadingMore && lastSlideInView

      if (loadMore) {
        listenForScrollRef.current = false

        //Load Logic here
        //setSlides(() => {})
      }

      return loadingMore || lastSlideInView
    })
  }, [])

  const addScrollListener = useCallback(
    (verticalISetCarouselAPI: CarouselApi) => {
      if (!verticalISetCarouselAPI) return
      scrollListenerRef.current = () => onScroll(verticalISetCarouselAPI)
      verticalISetCarouselAPI.on('scroll', scrollListenerRef.current)
    },
    [onScroll]
  )

  useEffect(() => {
    if (!verticalISetCarouselAPI) return
    addScrollListener(verticalISetCarouselAPI)

    const onResize = () => verticalISetCarouselAPI.reInit()
    window.addEventListener('resize', onResize)
    verticalISetCarouselAPI.on('destroy', () => window.removeEventListener('resize', onResize))
  }, [verticalISetCarouselAPI, addScrollListener])

  useEffect(() => {
    hasMoreToLoadRef.current = hasMoreToLoad
  }, [hasMoreToLoad])


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

      <Carousel setApi={setVerticalISetCarouselAPI} orientation="vertical" opts={{ align: "center", watchDrag: lockedAxis !== "horizontal",  watchSlides: watchslideLogic}} className="h-full ">
        <CarouselContent className="h-full w-full mt-0">
          {Array.from({ length: Math.min(currentIndex + 1 + 2, imageSets.length) }, (_, index) => (
            <CarouselItem className="basis-full" key={`imageSet-${imageSets[index]._id}`}>
              <ImageSetViewer set={imageSets[index]} distance={Math.abs(currentIndex - index)} DirectionLock={lockedAxis !== "vertical"} />
            </CarouselItem>
          ))}
        </CarouselContent>
      </Carousel>
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