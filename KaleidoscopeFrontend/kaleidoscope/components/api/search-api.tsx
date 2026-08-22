import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"



export interface SearchFilter {
  words: string[]
  tags: string[]
  titles: string[]
  authors: string[]
  sources: string[]
  searchTags: boolean
  searchTitles: boolean
  searchAuthors: boolean
  searchSources: boolean
  fromDate?: string
  toDate?: string
}

export interface SearchRequest extends SearchFilter {
  protectedApiRef: protectedAPI
  pageCount: number
  skipCount: number
  randomSeed?: string
}

export interface SetData {
  _id: string
  tags: string[]
  activeImageCount: number
  deleted?: boolean
}
export interface ImageIdsCountResponse {
  imageSets: SetData[]
  count: number
}


const verbose = false

export async function searchAPI(request: SearchRequest): Promise<{ status: number, errorString?: string, imageSets?: SetData[], count?: number }> {

  const body = {
    "words": request.words,
    "tags": request.tags,
    "titles": request.titles,
    "authors": request.authors,
    "sources": request.sources,
    "searchTags": request.searchTags,
    "searchTitles": request.searchTitles,
    "searchAuthors": request.searchAuthors,
    "searchSources": request.searchSources,
    "skip_count": request.skipCount,
    "page_count": request.pageCount,
    "random_seed": request.randomSeed || "",
    "fromDate": request.fromDate || "",
    "toDate": request.toDate || ""
  }

  const newRequest: GORequest = {
    endpoint: "/search",
    type: "POST",
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
