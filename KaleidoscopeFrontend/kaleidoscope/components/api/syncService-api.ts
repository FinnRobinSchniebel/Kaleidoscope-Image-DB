import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"



export default async function syncService_api(service: string, protectedApi: protectedAPI): Promise<boolean> {

    const newRequest: GORequest = {
            endpoint: `/service/${encodeURIComponent(service)}/sync`,
            type: "POST",
            header: { },
        }
        const {status, errorString, response} = await protectedApi.CallProtectedAPI(newRequest)
        return status == 202

}
