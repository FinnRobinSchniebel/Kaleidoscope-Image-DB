import { apiSendRequest, GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"

interface Props {
    form: FormData
    protectedApi: protectedAPI
}

export default async function  PostZip({ form, protectedApi }: Props) : Promise<{status: number, response: any, errorString: string | undefined}> {

    const newRequest: GORequest = {
        endpoint: `/uploadZip`,
        type: "POST",
        header: { },
        formData: form
    }
    const {status, errorString, response} = await protectedApi.CallProtectedAPI(newRequest)
    return {status, response, errorString}
}