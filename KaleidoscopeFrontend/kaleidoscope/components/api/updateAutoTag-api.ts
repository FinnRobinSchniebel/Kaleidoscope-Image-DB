import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"

interface Props {
  id: string
  name: string
  srcTagKeyMatch: string[]
  protectedApi: protectedAPI
}

export default async function updateAutoTag_api({ id, name, srcTagKeyMatch, protectedApi }: Props): Promise<{ conflict: true } | boolean> {

  const newRequest: GORequest = {
    endpoint: `/autotags/${encodeURIComponent(id)}`,
    type: "PATCH",
    header: { 'Content-Type': 'application/json' },
    body: { name, srcTagKeyMatch },
  }

  const { status, errorString } = await protectedApi.CallProtectedAPI(newRequest)
  if (status == 409) {
    return { conflict: true }
  }
  if (status != 200) {
    console.log(errorString)
    return false
  }

  return true
}
