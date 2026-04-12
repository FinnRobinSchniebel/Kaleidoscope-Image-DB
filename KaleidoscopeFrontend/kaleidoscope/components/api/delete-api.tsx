import { GORequest } from "./apicaller"
import { protectedAPI } from "./jwt_apis/protected-api-client"

interface Request{
    id : string
    protectedApi : protectedAPI
}


export async function deleteApi({id, protectedApi}: Request): Promise<boolean> {

  const newRequest: GORequest = {
    endpoint: `/ImageSets?ids=${id}`,
    type: "DELETE",
    header: { 'Content-Type': 'application/json' },
    
  }

  const {status, errorString, response} = await protectedApi.CallProtectedAPI(newRequest)
  if (status != 200){
    console.log(errorString)
    return false
  }


  return true
}



