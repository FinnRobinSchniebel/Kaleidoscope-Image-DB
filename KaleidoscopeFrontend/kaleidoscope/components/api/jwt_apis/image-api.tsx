import { GORequest } from "../apicaller"
import { protectedAPI } from "./protected-api-client"



export interface imageRequest {
  protectedApiRef: protectedAPI
  ID : string
  Index: number
  Lowres: boolean
}

export interface setData {
  id: string
  tags: string[]
}
export interface imageSetIDResponse {
  imageSets: setData[]
  count: number
}




export async function imageAPI(request: imageRequest): Promise<string> {

  const Params = {
    //TODO: from date and to Date
    image_set_id: request.ID || "",
    index: request.Index,
    Lowres: request.Lowres
  }

  const newRequest: GORequest = {
    endpoint: `/search?${Params.toString()}`,
    type: "Get",
    header: { 'Content-Type': 'application/json' },
    
  }

  const {status, errorString, response} = await request.protectedApiRef.CallProtectedAPI(newRequest)
  if (status != 200){
    console.log(errorString)
    return ""
  }

  if(!(response instanceof Blob)){
    return ""
  }

  return  URL.createObjectURL(response)
}
