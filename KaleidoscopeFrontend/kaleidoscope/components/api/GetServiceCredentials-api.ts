import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"

export interface ServiceCredentials {
  key1?: string
  key2?: string
  username?: string
  password?: string
  sync_interval_hours?: number
}

export async function getServiceCredentials(
  service: string,
  protectedApi: protectedAPI
): Promise<ServiceCredentials | null> {
  const request: GORequest = {
    endpoint: `/service/${encodeURIComponent(service)}/key`,
    type: "GET",
    header: {},
  }
  const { status, response } = await protectedApi.CallProtectedAPI(request)
  if (status !== 200) return null
  return response as ServiceCredentials
}
