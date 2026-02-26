'use client'
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { AlertCircleIcon, CheckCircle2, CheckCircle2Icon } from "lucide-react"
import { useState } from "react";


interface vars {
  code: number
  text: string
}

export default function LoginAlert({ code, text }: vars) {
  

  console.log(code)

  if(code == 200){
    return(
      <Alert>
        <CheckCircle2Icon/>
        <AlertTitle>{text}</AlertTitle>
        <AlertDescription>You will be redirected in a moment.</AlertDescription>
      </Alert>
    )
  }
  else if(code == 404){
    return (
      <Alert variant={"destructive"}>
        <AlertCircleIcon/>
        <AlertTitle>Could not Connect to server</AlertTitle>
        <AlertDescription>Wait for the servers to come online</AlertDescription>
      </Alert>
    )
  }
  else if(code > 299){
    return(
      <Alert variant={"destructive"}>
        <AlertCircleIcon/>
        <AlertTitle>{text}</AlertTitle>
        <AlertDescription>Please try again.</AlertDescription>
      </Alert>
    )
    
  }

  
}