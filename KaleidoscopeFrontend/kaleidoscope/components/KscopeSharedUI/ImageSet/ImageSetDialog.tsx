import { Carousel, CarouselContent, CarouselItem } from "@/components/ui/carousel";
import ImageSetViewer from "./ImageSetViewer";
import { DialogContent, DialogDescription, DialogHeader } from "@/components/ui/dialog";
import { DialogTitle } from "@radix-ui/react-dialog";
import VerticalImageSetCarousel from "./VerticalSetCarousel";
import { SetData } from "@/components/api/jwt_apis/search-api";


interface Props{
  imageSets: SetData[]
  index: number
}

export default function ImageSetDialog({imageSets, index} : Props){


    return (
        <DialogContent className='text-primary rounded-none h-dvh' OverlayClassName='bg-black/40 backdrop-blur-[2px]'  onInteractOutside={(e) => e.preventDefault()}>
          <DialogHeader hidden={true}>
            <DialogTitle> Imageset viewer Open</DialogTitle>
          </DialogHeader>
          <DialogDescription hidden>
            Contains the images of image set
          </DialogDescription>
          
          <VerticalImageSetCarousel imageSets={imageSets} index={index}/>
          
        </DialogContent>
        
    )



}