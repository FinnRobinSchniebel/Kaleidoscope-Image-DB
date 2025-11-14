

import { getServerAPI } from "./get_variables_server"


export const API_BASE = '/api'

export const AUTH_JWT = '/session'
export const AUTH_REGISTER = '/session/register'
export const AUTH_LOGIN = '/session/login'
export const AUTH_LOGOUT = '/session/logout'

export const API_IMAGESET = '/imagesets'
export const API_IMAGE = '/image'
export const API_SEARCH = '/search'

// export async function getServerAPI(endpoint : string) {
  
//   //console.log(process.env.NEXT_PUBLIC_BACKEND)
//   var env = process.env.BACKEND_URL
//   if (env != undefined){
//     console.log(env)
//     return 'http://'+  env + API_BASE + endpoint
//   }
//   else{
//     console.error("Backend URL:no config value provided: using default")
//     return "http://localhost:3005" + API_BASE + endpoint
//   }
// }

export interface GORequest {
  endpoint: string
  type: string;
  header: {};
  body?: JSON | string | undefined | object,
  media?: File[] | undefined
}


class GoApiError extends Error {
  status: number;

  constructor(status: number, errorString : string) {
    super(errorString); // sets Error.message
    this.status = status;
    this.message = errorString;
  }
}

export async function apiSendRequest(request : GORequest): Promise<{status: number, errorString?: string, response?: any}> {
  
  const options: RequestInit = {
    method: request.type,
    headers: request.header,    
    credentials: 'include',
    body: JSON.stringify(request.body)
  }
  console.log(getServerAPI(request.endpoint))

  try{
    const response = await fetch(await(getServerAPI(request.endpoint)), options)
    //console.log(response)
    
    if (!response.ok) {
      throw new GoApiError(response.status, await response.text());
    }
    const responseBody = {status: response.status, response: await response.json()};
 
    return responseBody
  
  } catch(error){

    if( error instanceof GoApiError){
      return {status: error.status, errorString: error.message }
    }
    if(error instanceof Error){
      return {status: 404, errorString: error.message }
    }     

    return {status: 300, errorString: "unknown error"}
  }
}



