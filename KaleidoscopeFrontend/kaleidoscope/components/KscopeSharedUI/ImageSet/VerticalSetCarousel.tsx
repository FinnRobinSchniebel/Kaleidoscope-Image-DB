import { Carousel, CarouselContent, CarouselItem } from "@/components/ui/carousel";
import ImageSetViewer from "./ImageSetViewer";
import { DialogContent, DialogDescription, DialogHeader } from "@/components/ui/dialog";
import { DialogTitle } from "@radix-ui/react-dialog";
import { SetData } from "@/components/api/jwt_apis/search-api";
import { useRef, useState } from "react";


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
            className="h-h-full w-full">

            <Carousel orientation="vertical" opts={{ align: "center",  watchDrag: lockedAxis !== "horizontal"}} className="bg-amber-400 flex justify-center text-primary h-full w-full ">
                <CarouselContent className="h-full w-full bg-amber-800">
                    <CarouselItem className="bg-pink-400">
                        <ImageSetViewer set={imageSets[index]} current={true} DirectionLock={lockedAxis !== "vertical"} />
                    </CarouselItem>
                </CarouselContent>
            </Carousel>
        </div>
    )


}