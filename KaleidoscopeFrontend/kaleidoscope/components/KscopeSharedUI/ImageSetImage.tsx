import Image from "next/image"
import { useProtected } from "../api/jwt_apis/ProtectedProvider"


interface props{
    Index : number
    SetID: string
}

export default function ImageSetImage({Index, SetID} : props){

    const protectedApi = useProtected()



    return(
        <>
            <Image src="" fill alt="">


            </Image>
        </>
    )
}