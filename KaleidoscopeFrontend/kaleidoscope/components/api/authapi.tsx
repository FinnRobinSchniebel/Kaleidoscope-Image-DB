"use client"

import { apiSendRequest, AUTH_LOGIN, AUTH_REGISTER, GORequest } from "@/components/api/apicaller"
import { permanentRedirect, redirect } from "next/navigation"
import { jwtDecode, JwtPayload } from "jwt-decode";
import {JWTLayout} from "./jwt_apis/protected-api-client"
import Cookies from "js-cookie"
import { TestToken } from "./get_variables_server";

const verbose = false

const TokenKey = "session_token"


interface Args {
    username : string,
    password : string
}

export async function CreateUser({username, password} : Args): Promise<{code : number, text: string}>{
    const request : GORequest = {
        endpoint: `${AUTH_REGISTER}?username=${username}&password=${password}` ,
        type: 'POST',
        header: {'Content-Type': 'application/json'},
        
    }
    
    var result = await apiSendRequest(request)
     
    if (result.status >=200 && result.status < 300) {
        return {code: result.status, text: "all good"}
    }
    
    return {code: result.status, text: result.errorString ?? ''}
}



export async function LoginUser({username, password} : Args): Promise<{code : number, text: string}>{
    
    //const params = {username: username, password : password}

    const request : GORequest = {
        endpoint: `${AUTH_LOGIN}?username=${username}&password=${password}` ,
        type: 'POST',
        header: {'Content-Type': 'application/json'},
        
    }
    
    var result = await apiSendRequest(request)
     
    if ("response" in result && TokenKey in result.response){
        //sessionStorage.setItem("session_token", result.response.session_token)
        const jwtFromMessage = result.response.session_token 

        const decoded= jwtDecode<JWTLayout>(jwtFromMessage) 
        if(verbose) console.log(JSON.stringify(decoded))
        
        Cookies.set('session_token', result.response.session_token, { expires: new Date((decoded.exp ?? 0) * 1000)})

         
        return {code: 200, text:'Login successful'}
    } 
    
    return {code: result.status, text: result.errorString ?? ''}
}

export async function NewSessionToken() : Promise<number | undefined>{
    const request : GORequest = {
        endpoint: `/session` ,
        type: 'GET',
        header: {'Content-Type': 'application/json'},
        
    }
    var result = await apiSendRequest(request)
    if ("response" in result && TokenKey in result.response){
        const decoded= jwtDecode<JWTLayout>(result.response.session_token) 
        Cookies.set('session_token', result.response.session_token, { expires: new Date((decoded.exp ?? 0) * 1000)})
    } 
    return result.status
    
}

//return true if the session token exists. 
//if it does not it will attempt to refresh it in case you have a valid refresh token. 
export async function TestLogin() : Promise<boolean>{
    if(await TestToken()){
        return true
    }
    const result = await NewSessionToken()

    return result == 200
}