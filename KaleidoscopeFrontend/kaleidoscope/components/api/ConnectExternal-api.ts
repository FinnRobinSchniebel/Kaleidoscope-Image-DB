import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"



export default async function connectExteranel_api(service: string, form: any, protectedApi : protectedAPI): Promise<boolean> {

    const newRequest: GORequest = {
            endpoint: `/service/${encodeURIComponent(service)}/register`,
            type: "POST",
            header: { },
            formData: form
        }
        const {status, errorString, response} = await protectedApi.CallProtectedAPI(newRequest)
        return status == 200

}