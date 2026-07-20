import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"



export default async function setSyncSchedule_api(service: string, syncIntervalHours: number, protectedApi: protectedAPI): Promise<boolean> {

    const formData = new FormData()
    formData.append("sync_interval_hours", String(syncIntervalHours))

    const newRequest: GORequest = {
            endpoint: `/service/${encodeURIComponent(service)}/syncSchedule`,
            type: "POST",
            header: { },
            formData: formData
        }
        const {status, errorString, response} = await protectedApi.CallProtectedAPI(newRequest)
        return status == 200

}
