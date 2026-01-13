import { Carousel, CarouselApi, CarouselContent, CarouselItem } from "@/components/ui/carousel";
import ImageSetViewer from "./ImageSetViewer";
import { DialogContent, DialogDescription, DialogHeader } from "@/components/ui/dialog";
import { DialogTitle } from "@radix-ui/react-dialog";
import { SetData } from "@/components/api/jwt_apis/search-api";
import { JSX, useLayoutEffect, useRef, useState } from "react";


interface Props {
    imageSets: SetData[]
    index: number
}

export default function VerticalImageSetCarousel({ imageSets, index }: Props) {


    const [lockedAxis, setLockedAxis] = useState<"horizontal" | "vertical" | null>(null)
    const startRef = useRef<{ x: number; y: number } | null>(null)
    // return (
    //     <ImageSetViewer set={imageSets[index]} current={true} />
    // )

    //ImageSet Index
    const [currentIndex, setCurrentIndex] = useState<number>(index)

    //Last carousel index
    const carouselIndexRef = useRef(0)
    const logicalIndexRef = useRef(index)
    //list of carousel items
    const [verticalSets, setVerticalSets] = useState<JSX.Element[]>([])

    //api to get info from the imagesets carousel
    const [verticalISetCarouselAPI, setVerticalISetCarouselAPI] = useState<CarouselApi>()



    useLayoutEffect(() => {


    }, [currentIndex])

    useLayoutEffect(() => {
        if (!verticalISetCarouselAPI) {
            return
        }
        const onSelect = () => {
            const newCarouselIndex = verticalISetCarouselAPI.selectedScrollSnap()
            const delta = newCarouselIndex - carouselIndexRef.current

            if (delta !== 0) {
                logicalIndexRef.current += delta
                carouselIndexRef.current = newCarouselIndex

                setCurrentIndex(logicalIndexRef.current)
            }
            console.log(`Cindex: ${carouselIndexRef.current}`)
            console.log(logicalIndexRef.current)
        }
        verticalISetCarouselAPI.on("select", onSelect)
        console.log(`Cindex: ${carouselIndexRef.current}`)
        console.log(logicalIndexRef.current)

        return () => {
            verticalISetCarouselAPI.off("select", onSelect)
        }
    }, [verticalISetCarouselAPI])
    

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

            <Carousel setApi={setVerticalISetCarouselAPI} orientation="vertical" opts={{ align: "center", watchDrag: lockedAxis !== "horizontal" }} className="h-full ">
                <CarouselContent className="h-full w-full mt-0">
                    <CarouselItem className="basis-full" key={`k1`}>
                        <ImageSetViewer set={imageSets[index]} current={true} DirectionLock={lockedAxis !== "vertical"} />
                    </CarouselItem>
                    <CarouselItem className=" basis-full" key={'k2'}>
                        <ImageSetViewer set={imageSets[index]} current={true} DirectionLock={lockedAxis !== "vertical"} />
                    </CarouselItem>
                    <CarouselItem className=" basis-full" key={'k3'}>
                        <ImageSetViewer set={imageSets[index]} current={true} DirectionLock={lockedAxis !== "vertical"} />
                    </CarouselItem>
                    <CarouselItem className=" basis-full" key={'k4'}>
                        <ImageSetViewer set={imageSets[index]} current={true} DirectionLock={lockedAxis !== "vertical"} />
                    </CarouselItem>
                </CarouselContent>
            </Carousel>
        </div>
    )


}