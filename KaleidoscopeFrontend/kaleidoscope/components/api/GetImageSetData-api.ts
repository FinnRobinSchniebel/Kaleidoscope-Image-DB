import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"


export interface FullImageSetData {
    Id: string
    Tags: string[]
    Title: string
    Authors: string[]
    Description: string
    DateAdded: string
    Sources: SourceInfo[]
    ActiveImageCount: number
    TagOverrides: string[]
}
export interface SourceInfo {
    Name: string
    Id: string
    SourceTitle: string
    SourceID: string
    SourceTags: string[]
}

//for api only
interface ApiSourceInfo {
  name: string
  id: string
  title: string
  sourceid: string
  tags: string[]
}

interface ApiImageSet {
  _id: string
  tags: string[]
  title: string
  authors: string[]
  description: string
  sources: ApiSourceInfo[]
  tag_rule_overrides: string[]
  activeImageCount: number
}


interface Props{
    id: string
    protectedApi: protectedAPI
}

function mapImageSet(api: ApiImageSet): FullImageSetData {
  return {
    Id: api._id,
    Tags: api.tags,
    Title: api.title,
    Authors: api.authors,
    Description: api.description,
    Sources: api.sources.map(s => ({
      Name: s.name,
      Id: s.id,
      SourceTitle: s.title,
      SourceID: s.sourceid,
      SourceTags: s.tags,
    })),
    ActiveImageCount: api.activeImageCount,
    TagOverrides: api.tag_rule_overrides,
    DateAdded: "" // backend doesn't provide this
  }
}


export default async function GetImageSetData({id, protectedApi} : Props) : Promise<FullImageSetData | undefined> {



    const newRequest: GORequest = {
        endpoint: `/getimagedata?ids=${id}`,
        type: "Get",
        header: { 'Content-Type': 'application/json' },
    }


    const { status, errorString, response } = await protectedApi.CallProtectedAPI(newRequest)
    if (status != 200) {
        console.log(errorString)
        return 
    }
    const apiInfo = response.imagesets?.[0]
    if (!apiInfo) return undefined

    return mapImageSet(apiInfo)
    


}