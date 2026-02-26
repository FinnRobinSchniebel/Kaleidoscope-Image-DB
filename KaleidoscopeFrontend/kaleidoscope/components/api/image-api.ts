import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"
import { SetData } from "./jwt_apis/search-api"



export interface imageRequest {
  protectedApiRef: protectedAPI
  ID : string
  Index: number
  Lowres: boolean
}

export interface imageSetIDResponse {
  imageSets: SetData[]
  count: number
}




export async function imageAPI(request: imageRequest): Promise<string> {


  const newRequest: GORequest = {
    endpoint: `/image?image_set_id=${request.ID || ""}&index=${request.Index}&lowres=${request.Lowres}`,
    type: "GET",
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

  return URL.createObjectURL(response)
}




export async function thumbNailAPI(request: imageRequest): Promise<string> {


  const newRequest: GORequest = {
    endpoint: `/thumbnail?id=${request.ID}`,
    type: "GET",
    header: { 'Content-Type': 'application/json' },
    
  }

  const {status, errorString, response} = await request.protectedApiRef.CallProtectedAPI(newRequest)
  if (status != 200){
    console.warn("Thumbnail fetch failed: ",errorString)
    return ""
  }

  const blob = response instanceof Blob ? response : new Blob([response], { type: 'image/png' })

  return URL.createObjectURL(blob)
}
