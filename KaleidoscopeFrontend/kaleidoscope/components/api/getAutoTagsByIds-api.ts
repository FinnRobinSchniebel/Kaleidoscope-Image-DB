import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"

export interface AutoTagSummary {
  Id: string
  Name: string
  Count: number
}

//for api only
interface ApiAutoTagSummary {
  id: string
  name: string
  count: number
}

interface Props {
  ids: string[]
  protectedApi: protectedAPI
}

function mapAutoTagSummary(api: ApiAutoTagSummary): AutoTagSummary {
  return {
    Id: api.id,
    Name: api.name,
    Count: api.count,
  } satisfies AutoTagSummary
}

export default async function getAutoTagsByIds_api({ ids, protectedApi }: Props): Promise<AutoTagSummary[] | undefined> {

  if (ids.length === 0) return []

  const newRequest: GORequest = {
    endpoint: `/autotags/byIds?ids=${ids.join(",")}`,
    type: "GET",
    header: { 'Content-Type': 'application/json' },
  }

  const { status, errorString, response } = await protectedApi.CallProtectedAPI(newRequest)
  if (status != 200) {
    console.log(errorString)
    return
  }

  const apiTags = response as ApiAutoTagSummary[]
  return Array.isArray(apiTags) ? apiTags.map(mapAutoTagSummary) : []
}
