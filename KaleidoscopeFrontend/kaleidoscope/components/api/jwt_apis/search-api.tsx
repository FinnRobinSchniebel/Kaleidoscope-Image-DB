import { GORequest } from "../apicaller"
import { protectedAPI } from "./protected-api-client"



export interface SearchRequest {
  protectedApiRef: protectedAPI
  tags?: string[]
  authors?: string[]
  titles?: string
  pageCount: number
  pageNumber: number
  randomSeed?: string
  fromDate?: string
  toDate?: string
}

export interface SetData {
  _id: string
  tags: string[]
  activeImageCount: number
}
export interface ImageIdsCountResponse {
  imageSets: SetData[]
  count: number
}


const verbose = false

export async function searchAPI(request: SearchRequest): Promise<{ status: number, errorString?: string, imageSets?: SetData[], count?: number }> {

  const body = {
    "tags": request.tags || [],
    "author": request.authors || [],
    "title": request.titles || "",
    "page": request.pageNumber,
    "page_count": request.pageCount,
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
    if(verbose) console.log("error " + errorString)
    return {status, errorString} 
  }



  return {status, imageSets: response.imagesets, count: response.totalCount} 


}
