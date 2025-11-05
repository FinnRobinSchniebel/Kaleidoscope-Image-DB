'use server'

import { use } from "react"

const API_BASE = '/api'

const AUTH_JWT = '/session'
const AUTH_REGISTER = '/session/register'
const AUTH_LOGIN = '/session/login'
const AUTH_LOGOUT = '/session/logout'

const API_IMAGESET = '/imagesets'
const API_IMAGE = '/image'
const API_SEARCH = '/search'

export async function getServerAPI(endpoint : string) {
  //console.log(process.env.NEXT_PUBLIC_BACKEND)
  var env = process.env.BACKEND_URL
  if (env != undefined){
    console.log(env)
    return 'http://'+  env + API_BASE + endpoint
  }
  else{
    console.error("Backend URL:no config value provided: using default")
    return "http://localhost:3005" + API_BASE + endpoint
  }
}

export interface GORequest {
  endpoint: string
  type: string;
  header: HeadersInit;
  body?: JSON | string | undefined,
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

export async function apiSendRequest(request : GORequest): Promise<any> {
  
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
    const responseBody =  {status: response.status, response: await response.json()};
   // cookieStore.set(response.)
    return responseBody
  
    //return {status: response.status, errorString: await response.text() }
  
  } catch(error: GoApiError| unknown){
    if( error instanceof GoApiError){
      return {status: error.status, errorString: error.message }
    }
     return {status: 404, errorString: 'could not reach server' }
  }
  
  
 
  
 
}

