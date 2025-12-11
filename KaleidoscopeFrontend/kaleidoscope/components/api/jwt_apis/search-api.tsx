import { GORequest } from "../apicaller"
import { protectedAPI } from "./protected-api-client"



export interface SearchRequest {
  protectedApiRef: protectedAPI
  tags?: string[]
  authors?: string[]
  Titles?: string
  PageCount: number
  PageNumber: number
  randomSeed?: string
  fromDate?: string
  toDate?: string
}

export interface SetData {
  _id: string
  tags: string[]
  active: number
}
export interface ImageIdsCountResponse {
  imageSets: SetData[]
  count: number
}




export async function searchAPI(request: SearchRequest): Promise<{ status: number, errorString?: string, imageSets?: SetData[], count?: number }> {

  const body = {
    "tags": request.tags || [],
    "author": request.authors || [],
    "title": request.Titles || "",
    "page": request.PageNumber,
    "page_count": request.PageCount,
    //TODO: from date and to Date
    "random_seed": request.randomSeed || "",
    "fromDate": request.fromDate || "",
    "toDate": request.toDate || ""
  }


  const newRequest: GORequest = {
    endpoint: "/search",
    type: "Post",
    header: { 'Content-Type': 'application/json' },
    body: body
  }

  const {status, errorString, response} = await request.protectedApiRef.CallProtectedAPI(newRequest)
  if (status != 200){
    console.log("error " + errorString)
    return {status, errorString} 
  }



  return {status, imageSets: response.imagesets, count: response.totalCount} 


}
