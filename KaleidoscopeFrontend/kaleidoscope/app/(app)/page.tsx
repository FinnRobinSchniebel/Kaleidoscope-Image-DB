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

  return (
    <div className='place-self-center '>Home
      <Button >Test button</Button>
    </div>
    
  );
}
