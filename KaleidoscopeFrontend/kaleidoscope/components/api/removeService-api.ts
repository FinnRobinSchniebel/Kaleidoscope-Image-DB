import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"



export default async function removeService_api(service: string, protectedApi: protectedAPI): Promise<{ success: boolean, error?: string }> {

    const newRequest: GORequest = {
            endpoint: `/service/${encodeURIComponent(service)}`,
            type: "DELETE",
            header: { },
        }
        const {status, errorString} = await protectedApi.CallProtectedAPI(newRequest)
        return { success: status == 200, error: errorString }

}
