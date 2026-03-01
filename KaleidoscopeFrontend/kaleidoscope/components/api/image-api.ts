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

export function ImageRequestToString(r : imageRequest): string {
  return `${r.ID}-${r.Index}-${r.Lowres}`
}


export async function imageAPI(request: imageRequest): Promise<{blob : Blob | null, err : string}> {

  const newRequest: GORequest = {
    endpoint: `/image?image_set_id=${request.ID || ""}&index=${request.Index}&lowres=${request.Lowres}`,
    type: "GET",
    header: { 'Content-Type': 'application/json' },
    
  }

  const {status, errorString, response} = await request.protectedApiRef.CallProtectedAPI(newRequest)
  if (status != 200){
    console.log(errorString)
    return {blob: null, err: errorString?? "error with return"}
  }

  if(!(response instanceof Blob)){
    return {blob: null, err: "Not a blob"}
  }

  return {blob: response, err: ""}
}




export async function thumbNailAPI(request: imageRequest): Promise<{blob : Blob | null, err : string}> {


  const newRequest: GORequest = {
    endpoint: `/thumbnail?id=${request.ID}`,
    type: "GET",
    header: { 'Content-Type': 'application/json' },
    
  }

  const {status, errorString, response} = await request.protectedApiRef.CallProtectedAPI(newRequest)
  if (status != 200){
    console.warn("Thumbnail fetch failed: ",errorString)
    return {blob: null, err: errorString?? "error with return"}
  }

  const blob = response instanceof Blob ? response : new Blob([response], { type: 'image/png' })

  return {blob: response, err: ""}
}
