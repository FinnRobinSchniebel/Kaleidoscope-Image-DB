import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"
import { SourceTagDoc } from "./getSourceTags-api"

export interface AutoTagWithMatches {
  Id: string
  Name: string
  Matches: SourceTagDoc[]
  Count: number
  System?: boolean
}

//for api only
interface ApiSourceTagDoc {
  key: string
  source: string
  tag: { default: string; en?: string }
  count: number
}

interface ApiAutoTagWithMatches {
  id: string
  name: string
  matches?: ApiSourceTagDoc[]
  count: number
  system?: boolean
}

interface Props {
  protectedApi: protectedAPI
}

function mapAutoTag(api: ApiAutoTagWithMatches): AutoTagWithMatches {
  return {
    Id: api.id,
    Name: api.name,
    Matches: (api.matches ?? []).map(m => ({
      Key: m.key,
      Source: m.source,
      Tag: { Default: m.tag.default, En: m.tag.en },
      Count: m.count,
    })),
    Count: api.count,
    System: api.system,
  } satisfies AutoTagWithMatches
}

export default async function getAutoTagDetails_api({ protectedApi }: Props): Promise<AutoTagWithMatches[] | undefined> {

  const newRequest: GORequest = {
    endpoint: `/autotags/details`,
    type: "GET",
    header: { 'Content-Type': 'application/json' },
  }

  const { status, errorString, response } = await protectedApi.CallProtectedAPI(newRequest)
  if (status != 200) {
    console.log(errorString)
    return
  }

  const apiTags = response as ApiAutoTagWithMatches[]
  return Array.isArray(apiTags) ? apiTags.map(mapAutoTag) : []
}
