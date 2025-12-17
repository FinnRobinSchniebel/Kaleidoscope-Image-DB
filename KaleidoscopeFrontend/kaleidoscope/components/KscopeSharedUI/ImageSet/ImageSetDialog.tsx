import { Carousel, CarouselContent, CarouselItem } from "@/components/ui/carousel";
import ImageSetViewer from "./ImageSetViewer";
import { DialogContent, DialogDescription, DialogHeader } from "@/components/ui/dialog";
import { DialogTitle } from "@radix-ui/react-dialog";
import VerticalImageSetCarousel from "./VerticalSetCarousel";


export default function ImageSetDialog(){


    return (
        <DialogContent className='text-primary rounded-none h-dvh' OverlayClassName='bg-black/40 backdrop-blur-[2px]'  onInteractOutside={(e) => e.preventDefault()}>
          <DialogHeader hidden={true}>
            <DialogTitle> Imageset viewer Open</DialogTitle>
          </DialogHeader>
          <DialogDescription hidden>
            Contains the images of image set
          </DialogDescription>
          <VerticalImageSetCarousel/>
        </DialogContent>
        
    )



}