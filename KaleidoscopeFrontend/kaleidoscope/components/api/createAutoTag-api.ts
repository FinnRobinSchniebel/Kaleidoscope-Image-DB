import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"

interface Props {
  name: string
  srcTagKeyMatch: string[]
  protectedApi: protectedAPI
}

export default async function createAutoTag_api({ name, srcTagKeyMatch, protectedApi }: Props): Promise<{ id: string } | { conflict: true } | undefined> {

  const newRequest: GORequest = {
    endpoint: `/autotags`,
    type: "POST",
    header: { 'Content-Type': 'application/json' },
    body: { name, srcTagKeyMatch },
  }

  const { status, errorString, response } = await protectedApi.CallProtectedAPI(newRequest)
  if (status == 409) {
    return { conflict: true }
  }
  if (status != 200) {
    console.log(errorString)
    return
  }

  return { id: response.id as string }
}
