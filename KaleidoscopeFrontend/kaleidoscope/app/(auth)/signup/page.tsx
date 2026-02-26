'use client'

import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useRouter, useSearchParams } from "next/navigation";

import { useEffect, useState } from "react";
import LoginAlert from "../login/loginalert";
import { CreateUser, LoginUser } from "@/components/api/authapi";
import Link from "next/link";



export default function Register() {
  const router = useRouter()
  const params = useSearchParams()


  const searchParams = useSearchParams()

  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');

  const [alertBox, setAlertBox] = useState({ code: 0, text: '' });

  const handleChangeUser = (event: React.ChangeEvent<HTMLInputElement>) => {
    setUsername(event.target.value);
  };
  const handleChangePass = (event: React.ChangeEvent<HTMLInputElement>) => {
    setPassword(event.target.value);
  };

  const handleSubmit = async () => {
    var pair = await CreateUser(({ username, password }))

    if (pair.code >= 200 && pair.code < 300){
      pair = await LoginUser(({ username, password }))
    }

    setAlertBox({ code: pair.code, text: pair.text })
    //await sleep(500)

    if (pair.code == 200) {

      router.push(searchParams.get('from') ?? '/')
      //ServerRedirect()
    }

  }



  return (
    <Card className="bg-foreground w-100 border-white/30 text-primary font-bold backdrop-blur-sm shadow-xl/30">
      <CardHeader>
        <CardTitle className="font-bold text-2xl"><h1>Create an Account</h1></CardTitle>
        <CardDescription className="text">
          Enter your username below to login to create a new account.
        </CardDescription>
        <CardAction>
          <Button asChild variant="link" className="text-base text-blue-600 underline hover:decoration-3 "  >
            <Link href="/login">
              Login
            </Link>
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        <form>
          <div className="flex flex-col gap-6">
            <div className="grid gap-2">
              <Label htmlFor="email">User</Label>
              <Input
                id="signup-username"
                // type="email"
                placeholder="enter user name"
                onChange={handleChangeUser}
                required
              />
            </div>
            <div className="grid gap-2">
              <div className="flex items-center">
                <Label htmlFor="password">Password</Label>
              </div>
              <Input
                id="signup-password"
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
        <Button type="submit" className="w-full" onClick={handleSubmit}>
          Create account
        </Button>
        <LoginAlert code={alertBox.code} text={alertBox.text} />
      </CardFooter>
    </Card>
  );


}

