'use server'

import { permanentRedirect, redirect } from "next/navigation"
import { API_BASE } from "./apicaller"
import { cookies } from "next/headers"



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



export async function ServerRedirect(to : string){
  'use server'
  console.log('test--------------------------------------')
  redirect(to)
}



export async function ReadToken(){  
  const t = (await cookies())
  // console.log("TTTTTTTTTTTTTTTTT  " + t.get('session_token'))

  return t.get('session_token')?.value ?? ''
}

export async function TestToken(){  
  const t = (await cookies())
  // console.log("TTTTTTTTTTTTTTTTT  " + t.get('session_token'))

  return t.has('session_token')
}