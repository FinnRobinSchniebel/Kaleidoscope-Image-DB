import { Carousel, CarouselContent, CarouselItem, CarouselNext, CarouselPrevious } from "@/components/ui/carousel";




export default function ImageSetViewer() {


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