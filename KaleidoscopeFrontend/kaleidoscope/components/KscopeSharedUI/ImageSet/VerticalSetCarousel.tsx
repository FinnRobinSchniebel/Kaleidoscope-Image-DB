import { Carousel, CarouselContent, CarouselItem } from "@/components/ui/carousel";
import ImageSetViewer from "./ImageSetViewer";
import { DialogContent, DialogDescription, DialogHeader } from "@/components/ui/dialog";
import { DialogTitle } from "@radix-ui/react-dialog";


export default function VerticalImageSetCarousel() {


    return (
        <ImageSetViewer />
    )

    // return (
    //     <>
    //     <Carousel>
    //         <CarouselContent>
    //             <CarouselItem>
    //                 <ImageSetViewer/>
    //             </CarouselItem>
    //         </CarouselContent>
    //     </Carousel>
    //     </>
    // )


}