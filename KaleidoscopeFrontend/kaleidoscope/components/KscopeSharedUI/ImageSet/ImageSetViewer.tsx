import GetImageSetData, { FullImageSetData } from "@/components/api/GetImageSetData-api";
import { useProtected } from "@/components/api/jwt_apis/ProtectedProvider";
import { SetData } from "@/components/api/jwt_apis/search-api";
import { Carousel, CarouselContent, CarouselItem, CarouselNext, CarouselPrevious } from "@/components/ui/carousel";
import { useEffect, useState } from "react";





interface Props {
  set: SetData
}


export default function ImageSetViewer(props: Props) {

  console.log(props.set)

  const protectedApi = useProtected()
  const [ImageSetInfo, SetImageSetInfo] = useState<FullImageSetData>()

  useEffect(() => {
    const getData = async () => {
      SetImageSetInfo( await GetImageSetData({ id: props.set._id, protectedApi: protectedApi}))
    }
    getData()

  }, [props.set])

  //TODO: get index from url
  const [CurrentIndex, setCurrentIndex] = useState(0)



  return (
    <>
      <Carousel className="text-primary">
        <CarouselContent>

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