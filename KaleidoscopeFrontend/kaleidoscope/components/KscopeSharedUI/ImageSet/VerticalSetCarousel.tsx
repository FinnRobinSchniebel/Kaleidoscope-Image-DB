import { Carousel, CarouselContent, CarouselItem } from "@/components/ui/carousel";
import ImageSetViewer from "./ImageSetViewer";
import { DialogContent, DialogDescription, DialogHeader } from "@/components/ui/dialog";
import { DialogTitle } from "@radix-ui/react-dialog";
import { SetData } from "@/components/api/jwt_apis/search-api";


interface Props{
    imageSets: SetData[]
    index : number
}

export default function VerticalImageSetCarousel({imageSets, index} : Props) {


    return (
        <ImageSetViewer set={imageSets[index]} />
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