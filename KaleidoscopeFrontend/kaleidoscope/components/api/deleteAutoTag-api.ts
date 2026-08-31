import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"

interface Props {
  id: string
  protectedApi: protectedAPI
}

export default async function deleteAutoTag_api({ id, protectedApi }: Props): Promise<boolean> {

  const newRequest: GORequest = {
    endpoint: `/autotags/${encodeURIComponent(id)}`,
    type: "DELETE",
    header: { 'Content-Type': 'application/json' },
  }

  const { status, errorString } = await protectedApi.CallProtectedAPI(newRequest)
  if (status != 200) {
    console.log(errorString)
    return false
  }

  return true
}
