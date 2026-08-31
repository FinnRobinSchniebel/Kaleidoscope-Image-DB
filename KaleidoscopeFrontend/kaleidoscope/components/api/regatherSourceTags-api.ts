import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"

export interface RegatherSummary {
  scanned_sets: number
  scanned_sources: number
  distinct_tags: number
  created: unknown[]
  count_corrected: unknown[]
  translation_corrected: unknown[]
}

interface Props {
  protectedApi: protectedAPI
}

export default async function regatherSourceTags_api({ protectedApi }: Props): Promise<RegatherSummary | undefined> {

  const newRequest: GORequest = {
    endpoint: `/sourcetags/regather`,
    type: "POST",
    header: { 'Content-Type': 'application/json' },
  }

  const { status, errorString, response } = await protectedApi.CallProtectedAPI(newRequest)
  if (status != 200) {
    console.log(errorString)
    return
  }

  // Go serializes a nil slice as JSON null, not [] - normalize so callers
  // can rely on .length without checking each field first.
  const summary = response as RegatherSummary
  return {
    ...summary,
    created: summary.created ?? [],
    count_corrected: summary.count_corrected ?? [],
    translation_corrected: summary.translation_corrected ?? [],
  }
}
