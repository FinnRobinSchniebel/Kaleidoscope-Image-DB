'use client'

import { jwtDecode, JwtPayload } from "jwt-decode";
import { apiSendRequest, GORequest } from "../apicaller";
import { NewSessionToken } from "../authapi";
import { ReadToken } from "../get_variables_server";

export interface JWTLayout extends JwtPayload {
  Id: string
  SessionID: string
  //RefreshToken: JwtPayload
  IndefiniteRef: boolean
}

export interface SearchItems {
  //_id: string
  Author?: string[]
  Tags?: string[]
  FromDate?: number
  ToDate?: number
  Title?: string
}

export class protectedAPI {


  static token = ''
  //true while fetching new token
  private fetchingNewToken = false


  constructor(CurrentToken : string){
    protectedAPI.token = CurrentToken
  }

  //This halts the api call if another api has already requested a new token but has not received it yet.
  //It prevents unnecessary API calls and makes sure all waiting api's still run after a successful token update
  private async WaitForToken(): Promise<void> {
    return new Promise((resolve) => {
      const check = setInterval(() => {
        if (!this.fetchingNewToken) {
          clearInterval(check);
          resolve();
        }
      }, 50);
    });
  }


  //Used to check if the token has expired
  //returns true if expired | returns false if valid
  private CheckTokenExpired(token: string): boolean {
    const decoded = jwtDecode<JWTLayout>(token)
    const currentDate = new Date()
    console.log(decoded)
    if (decoded.exp != undefined && decoded.exp * 1000 > currentDate.getTime()) {
      console.log("valid token")
      return false;
    }
    return true;
  }

  //checks if The API call is good to run and refresh token if not
  private async CheckIfReady(): Promise<{ status: number, response: any }> {
    if (this.fetchingNewToken) {
      await this.WaitForToken()
    }
    if (protectedAPI.token == "" || this.CheckTokenExpired(protectedAPI.token)) {
      console.log("session expired")
      this.fetchingNewToken = true
      //Note: function below can only run on client
      const status = await NewSessionToken()
      //if token cant be refreshed error response so the user redirects to login
      if (status == 404 || status == 401) {
        return { status: status, response: "error" }
      }
      protectedAPI.token = await ReadToken()
      this.fetchingNewToken = false
      return { status: 200, response: "good" }
    }
    return { status: 0, response: "no change" }
  }

  //Use this function to call API's that require the session token.
  public async CallProtectedAPI(request: GORequest): Promise<{ status: number, errorString?: string, response?: any }> {

    // check if updating token
    var result = await this.CheckIfReady();
    if (result.status != 200 && result.status != 0) {
      return result;
    }
    //pre-check complete: do api call
    request.header = { ...request.header, ...{ "session_token": "Bearer " + protectedAPI.token } };

    var result2 = await apiSendRequest(request)

    //check if result is good and attempt to fix it if auth has expired
    if (result2.status == 401) {
      console.log("call failed: session expired")
      result = await this.CheckIfReady();
      //only rerun if the cookie expired (not 0 response) and was successfully refreshed.
      if (result.status == 0 || result.status != 200) {
        return result;
      }
      result2 = await apiSendRequest(request)
    }
    //return result after second attempt 
    return result2
  }


  public async GetSearch(Terms: SearchItems) {
    const newRequest: GORequest = {
      endpoint: "/search",
      type: "Post",
      header: { 'Content-Type': 'application/json'},
      body: JSON.stringify(Terms)
    }
    return this.CallProtectedAPI(newRequest)
  }
}