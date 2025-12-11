

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
  body?:string | undefined | object,
  media?: File[] | undefined
}

type FetchResponse =
  | { status: number; response: Blob }
  | { status: number; response: any }


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

  try{
    console.log("doing fetch")

    const path = await getServerAPI(request.endpoint)
    console.log("got path")
    const response = await fetch(path, options)
    console.log("finished fetch")
    
    if (!response.ok) {
      throw new GoApiError(response.status, await response.text());
    }
    //check of blob
    const contentType = response.headers.get("content-type") || "";

    let responseBody : FetchResponse

    if(contentType.startsWith("image/")){
      responseBody = {status: response.status, response: await response.blob()};
    }
    else{
      responseBody = {status: response.status, response: await response.json()};
    }
    
    console.log("fetch complete ... no errors")
    return responseBody
    
  } catch(error){
    console.log("fetch error")
    if( error instanceof GoApiError){
      return {status: error.status, errorString: error.message }
    }
    if(error instanceof Error){
      return {status: 404, errorString: error.message }
    }     

    return {status: 300, errorString: "unknown error"}
  }
}



