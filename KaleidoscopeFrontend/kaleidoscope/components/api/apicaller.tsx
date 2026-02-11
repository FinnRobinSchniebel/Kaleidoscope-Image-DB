

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


const verbose = false

export interface GORequest {
  endpoint: string
  type: "GET" | "POST" | "PUT" | "DELETE";
  header: {};
  body?: string | undefined | object,
  media?: File[] | undefined
  formData?: FormData
}

type FetchResponse =
  | { status: number; response: Blob }
  | { status: number; response: any }


class GoApiError extends Error {
  status: number;

  constructor(status: number, errorString: string) {
    super(errorString); // sets Error.message
    this.status = status;
    this.message = errorString;
  }
}

export async function apiSendRequest(request: GORequest): Promise<{ status: number, errorString?: string, response?: any }> {

  const options: RequestInit = {
    method: request.type,
    headers: request.header,
    credentials: 'include',
    //body: JSON.stringify(request.body)
  }
  if (request.formData) {
    options.body = request.formData
    delete (options.headers as any)?.["Content-Type"]
  }
  else {
    options.body = JSON.stringify(request.body)
  }


  try {
    if (verbose) console.log("doing fetch")

    const path = await getServerAPI(request.endpoint)
    if (verbose) console.log("got path")

    const response = await fetch(path, options)
    if (verbose) console.log("finished fetch")

    if (!response.ok) {
      throw new GoApiError(response.status, await response.text());
    }
    //check of blob
    const contentType = response.headers.get("content-type") || "";

    let responseBody: FetchResponse

    if (contentType.startsWith("image/")) {
      responseBody = { status: response.status, response: await response.blob() };
    }
    else {
      const text = await response.text()
      if (text) {
        try {
          responseBody = { status: response.status, response: JSON.parse(text) }
        } catch {
          // fallback if not valid JSON
          responseBody = { status: response.status, response: text }
        }
      } else {
        responseBody = { status: response.status, response: null } // empty response
      }
    }

    if (verbose) console.log("fetch complete ... no errors")
    return responseBody

  } catch (error) {
    if (verbose) console.log("fetch error")
    if (error instanceof GoApiError) {
      return { status: error.status, errorString: error.message }
    }
    if (error instanceof Error) {
      return { status: 404, errorString: error.message }
    }

    return { status: 300, errorString: "unknown error" }
  }
}



