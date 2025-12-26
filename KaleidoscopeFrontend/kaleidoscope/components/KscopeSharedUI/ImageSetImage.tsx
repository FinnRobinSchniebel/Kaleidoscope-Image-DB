import Image from "next/image"
import { useProtected } from "../api/jwt_apis/ProtectedProvider"
import { CarouselItem } from "../ui/carousel"
import { imageAPI, imageRequest } from "../api/image-api"
import { useEffect, useMemo, useState } from "react"


interface props {
    Index: number
    SetID: string
}

export default function ImageSetImage({ Index, SetID }: props) {

    const protectedApi = useProtected()

    const [image, setImage] = useState<string>("")

    var request: imageRequest = useMemo(() => ({
        protectedApiRef: protectedApi,
        ID: SetID,
        Index: Index,
        Lowres: true
    }), [SetID, Index])

    useEffect(() => {
        const t = async () => {
          setImage(await imageAPI(request))
        }
        t()
      }, [SetID, Index])


    return (
        <CarouselItem>
            <Image src={image} fill alt="">


            </Image>
        </CarouselItem>
    )
}