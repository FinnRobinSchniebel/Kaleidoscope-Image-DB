"use client"

import { NewSessionToken } from "@/components/api/authapi";
import { ReadToken } from "@/components/api/get_variables_server";
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { Button } from "@/components/ui/button";
import Image from "next/image";
import { useEffect } from "react";


export default function Home() {


  useEffect(() => {
    NewSessionToken()
  })

  const buttonfunc = async () =>{
    const Protected = new protectedAPI(await ReadToken());
    const result = await Protected.GetSearch({})
    console.log(result.status + " || " + JSON.stringify((result).response) + "||" + result.errorString)
  }

  return (
    <div className='place-self-center '>Home
      <Button onClick={buttonfunc}>Test button</Button>
    </div>
    
  );
}
