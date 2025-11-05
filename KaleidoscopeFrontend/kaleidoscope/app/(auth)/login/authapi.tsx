`use server`

import { apiSendRequest, GORequest } from "@/app/apicaller"
import { permanentRedirect, redirect } from "next/navigation"

const TokenKey = "session_token"


interface Args {
    username : string,
    password : string
}

export async function LoginUser({username, password} : Args): Promise<{code : number, text: string}>{
    
    //const params = {username: username, password : password}

    const request : GORequest = {
        endpoint: `/session/login?username=${username}&password=${password}` ,
        type: 'Post',
        header: {'Content-Type': 'application/json'},
        
    }
    
    var result = await apiSendRequest(request)
    console.log(result)
     
    if ("response" in result && TokenKey in result.response){
        console.log(result.response.session_token)
        localStorage.setItem("session_token", result.response.session_token)
        return {code: 200, text:'Login successful'}
        //NewSessionToken()
    } 
    else if ("status" in result){
        return {code: result.status, text: result.errorString}
    }
    else{
       return {code: result.status, text: result.errorString}
    }

    
}

export async function LoginRedirect(redirectpath : string | null){
    
    //const redirectpath = searchParams.get('from')\\
    console.log("fffffffff ", redirectpath)
    permanentRedirect("/")
}

async function NewSessionToken(){
    const request : GORequest = {
        endpoint: `/session` ,
        type: 'Get',
        header: {'Content-Type': 'application/json'},
        
    }
    var result = await apiSendRequest(request)
    console.log(result)
    if ("response" in result && TokenKey in result.response){
        console.log(result.response.session_token)
        localStorage.setItem("session_token", result.response.session_token)
    } 
}