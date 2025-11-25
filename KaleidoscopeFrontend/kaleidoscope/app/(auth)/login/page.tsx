'use client'

import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@radix-ui/react-label";
import { use, useEffect, useState } from "react";
import {LoginUser, NewSessionToken, TestLogin} from "@/components/api/authapi"
import LoginAlert from "./loginalert"
import { redirect, useSearchParams } from "next/navigation";
import { useRouter } from "next/navigation";
import { ReadToken, ServerRedirect } from "@/components/api/get_variables_server";


const sleep = (ms: number) => new Promise(resolve => setTimeout(resolve, ms))

export default function Login() {

  const router = useRouter()

  const searchParams = useSearchParams()

  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');

  const [alertBox, setAlertBox] = useState({code: 0, text: ''});

  const handleChangeUser = (event: React.ChangeEvent<HTMLInputElement>) => {
    setUsername(event.target.value);
  };
   const handleChangePass = (event: React.ChangeEvent<HTMLInputElement>) => {
    setPassword(event.target.value);
  };

  const handleSubmit = async () =>{
    const pair = await (LoginUser({username, password}))

    setAlertBox({code:pair.code, text: pair.text})
    //await sleep(500)

    if (pair.code == 200){

      router.push(searchParams.get('from') ?? '/')
      //ServerRedirect()
    }
   
  }
  
  useEffect( () => {
    const t = async ()=>{  
      const result = await TestLogin()
      const redirectpath = searchParams.get('from')
      router.push(redirectpath ?? "/")
    }
    t()
  })

  return (
    <Card className="bg-foreground w-100 border-white/30 text-primary font-bold backdrop-blur-sm shadow-xl/30">
      <CardHeader>
        <CardTitle className="font-bold text-2xl">Login to your account</CardTitle>
        <CardDescription className="text">
          Enter your username below to login to your account
        </CardDescription>
        <CardAction>
          <Button variant="link" className="text-base text-blue-600 underline hover:decoration-3 ">Sign Up</Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        <form>
          <div className="flex flex-col gap-6">
            <div className="grid gap-2">
              <Label htmlFor="email">User</Label>
              <Input
                id="username"
                // type="email"
                placeholder="enter user name"
                onChange={handleChangeUser}
                required
              />
            </div>
            <div className="grid gap-2">
              <div className="flex items-center">
                <Label htmlFor="password">Password</Label>
                <a
                  href="#"
                  className="ml-auto inline-block text-sm underline-offset-4 hover:underline"
                >
                  Forgot your password?
                </a>
              </div>
              <Input 
                id="password" 
                type="password" 
                onChange={handleChangePass} 
                placeholder="password"
                required 
              />
            </div>
          </div>
        </form>
      </CardContent>
      <CardFooter className="flex-col gap-2">
        <Button type="submit" className="w-full" onClick={handleSubmit} >
          Login
        </Button>     
        <LoginAlert code={alertBox.code} text={alertBox.text} />
      </CardFooter>
    </Card>
  );
}
