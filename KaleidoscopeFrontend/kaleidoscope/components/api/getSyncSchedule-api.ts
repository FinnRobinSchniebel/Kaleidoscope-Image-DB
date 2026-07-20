import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"

export interface ServiceSyncInfo {
  sync_interval_hours?: number
  last_synced?: string
}

export default async function getSyncSchedule_api(service: string, protectedApi: protectedAPI): Promise<ServiceSyncInfo | null> {

    const newRequest: GORequest = {
            endpoint: `/service/${encodeURIComponent(service)}/syncSchedule`,
            type: "GET",
            header: { },
        }
        const {status, response} = await protectedApi.CallProtectedAPI(newRequest)
        if (status != 200) return null
        return response as ServiceSyncInfo

}
