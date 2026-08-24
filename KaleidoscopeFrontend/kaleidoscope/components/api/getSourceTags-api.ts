import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"

export interface SourceTagDoc {
  Key: string
  Source: string
  Tag: { Default: string; En?: string }
  Count: number
}

//for api only
interface ApiSourceTagDoc {
  key: string
  source: string
  tag: { default: string; en?: string }
  count: number
}

interface Props {
  protectedApi: protectedAPI
}

export function displaySourceTagText(tag: SourceTagDoc): string {
  return tag.Tag.En || tag.Tag.Default
}

function mapSourceTag(api: ApiSourceTagDoc): SourceTagDoc {
  return {
    Key: api.key,
    Source: api.source,
    Tag: { Default: api.tag.default, En: api.tag.en },
    Count: api.count,
  } satisfies SourceTagDoc
}

export default async function getSourceTags_api({ protectedApi }: Props): Promise<SourceTagDoc[] | undefined> {

  const newRequest: GORequest = {
    endpoint: `/sourcetags`,
    type: "GET",
    header: { 'Content-Type': 'application/json' },
  }

  const { status, errorString, response } = await protectedApi.CallProtectedAPI(newRequest)
  if (status != 200) {
    console.log(errorString)
    return
  }

  const apiTags = response as ApiSourceTagDoc[]
  return Array.isArray(apiTags) ? apiTags.map(mapSourceTag) : []
}
