import { useProtected } from "@/components/api/jwt_apis/ProtectedProvider";
import { Carousel, CarouselContent, CarouselItem, CarouselNext, CarouselPrevious } from "@/components/ui/carousel";



interface Props{
    Id: string
}


export default function ImageSetViewer(props: Props) {




    return (
        <>

            <Carousel className="text-primary">
                <CarouselContent>
                    <CarouselItem>test</CarouselItem>
                    <CarouselItem>test2</CarouselItem>
                </CarouselContent>
                <CarouselPrevious />
                <CarouselNext />
            </Carousel>
            {/* image count */}
            {/* image slider */}
            {/* tags */}
            {/* discription */}


        </>

    )

}