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

  const hasloaded = useRef(false)


  //lowest ISet Index rendered (Logical)
  const [lowestIndex, setLowestIndex] = useState<number>(Math.max(setIndex - 2, 0))
  //Highest ISet Index rendered (Logical)
  const [highestIndex, setHighestIndex] = useState<number>(Math.min(setIndex + 2, imageSets.length))
  //ImageSet Index logical
  const LogicalIndex = useRef(setIndex)


  //ImageSet Index, Relative to carousel NOT logic
  const currentIndex = useRef(setIndex - lowestIndex)

  //api to get info from the imagesets carousel
  const [verticalISetCarouselAPI, setVerticalISetCarouselAPI] = useState<CarouselApi>()

  const [loadingMore, setLoadingMore] = useState(false)
  const listenForScrollRef = useRef(true)
  const hasMoreToLoadRef = useRef(true)
  const scrollListenerRef = useRef(() => undefined)
  const [hasMoreToLoad, setHasMoreToLoad] = useState(true)

  //ref to determine howmuch movement occured infront of the list
  const frontMovement = useRef(0)


  useLayoutEffect(() => {
    if (!verticalISetCarouselAPI) {
      return
    }
    const onSelect = () => {
      const newCarouselIndex = verticalISetCarouselAPI.selectedScrollSnap()
      currentIndex.current = newCarouselIndex
      LogicalIndex.current = lowestIndex + newCarouselIndex
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


      const oldIndex = currentIndex.current

      //currentIndex.current = frontMovement.current + oldIndex
      console.log("engien pos before "  + oldEngine.location.get())

      console.log("NEW LOW: " + lowestIndex + " NEW INDEX AFTER RELOAD " + currentIndex.current + " movement: " + frontMovement.current + " old Index: " + oldIndex)

      verticalISetCarouselAPI.scrollTo((LogicalIndex.current - lowestIndex), false)

      //newEngine.index.set(oldEngine.index.get()+ frontMovement.current)
      // newEngine.translate.to(frontMovement.current + oldEngine.location.get())
      //frontMovement.current = 0

      //const { index } = newEngine.scrollTarget.byDistance(0, false)
      // newEngine.index.set(index)
      //newEngine.animation.start()


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
    }
    // else {
    //   reloadEmbla() 
    // }
  }

  const onScroll = useCallback((verticalISetCarouselAPI: CarouselApi) => {

    console.log("engine pos before "  + verticalISetCarouselAPI?.internalEngine().location.get() + " slide size " + verticalISetCarouselAPI?.internalEngine().slideRects[0].height)
    if (!listenForScrollRef.current) return undefined


    //by performing this action in a set-state I can avoid race conditions 
    setLoadingMore((loadingMore) => {
      if (!verticalISetCarouselAPI) return false

      console.log(`checking if load more + ${loadingMore} +  ${verticalISetCarouselAPI?.slidesInView()} + ${currentIndex.current} + ${lowestIndex != 0}`)

      const lastSlide = verticalISetCarouselAPI.slideNodes().length - 1
      const AtCarouselEnd = verticalISetCarouselAPI.slidesInView().includes(lastSlide)
      const loadMoreEnd = !loadingMore && AtCarouselEnd && (highestIndex != imageSets.length - 1)

      const AtCarouselFront = verticalISetCarouselAPI.slidesInView().includes(0)
      const LoadMoreFront = !loadingMore && currentIndex.current === 0
      //console.log(`checking if load more + ${loadingMore} +  ${AtCarouselEnd}`)
      
      //Load Logic here
      if (loadMoreEnd) {
        console.log("loadMore back")
        listenForScrollRef.current = false
        setHighestIndex((highestIndex) => {
          return Math.min(highestIndex + 2, imageSets.length)
        })
      }

      if (LoadMoreFront) {
        console.log("loadMore front")
        listenForScrollRef.current = false
        const movement = Math.min(2, lowestIndex)
        frontMovement.current = movement

        console.log("movement: " + frontMovement.current)
        setLowestIndex((lowestIndex) => {
          return Math.max(lowestIndex - movement, 0)
        }
        )
      }

      return loadingMore || AtCarouselEnd || AtCarouselFront
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

  //set location of opened image
  useLayoutEffect(() => {
    if (hasloaded.current) return
    if (verticalISetCarouselAPI) hasloaded.current = true
    console.log("NEW OPEN")
    verticalISetCarouselAPI?.scrollTo(setIndex - lowestIndex, true)
  }, [setIndex, verticalISetCarouselAPI])


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
      {/* watchSlides: watchslideLogic  */}
      <Carousel setApi={setVerticalISetCarouselAPI} orientation="vertical" opts={{ align: "center", watchDrag: lockedAxis !== "horizontal",  }} className="h-full ">
        <CarouselContent className="h-full w-full mt-0">
          {imageSets.slice(lowestIndex, highestIndex).map((set, index) => (
            <CarouselItem className="basis-full" key={`imageSet-${set._id}`}>
              <ImageSetViewer set={set} distance={Math.abs(currentIndex.current - index)} DirectionLock={lockedAxis !== "vertical"} />
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