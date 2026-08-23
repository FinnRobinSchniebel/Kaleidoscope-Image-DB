'use client'

import { MenuButtonProps } from "@/components/KscopeSharedUI/account/IconButtonsMenu";



import { useState } from "react";

import AlertPopup from "@/components/KscopeSharedUI/ImageSet/AlertPopup";
import { Button } from "@/components/ui/button";
import Link from "next/link";
import { ChevronLeft } from "lucide-react";




interface Props {

}


export default function page({ }: Props) {


  

  return (
    <>

      <h1 className='p-10 text-4xl'>Tag Manager</h1>

      <div className='flex flex-col flex-1 w-full'>
         <Button className='m-4 w-fit bg-accent shadow-primary/60 hover:bg-accent/30' variant='outline' asChild>
          <Link href={`/account`}>
            <ChevronLeft></ChevronLeft>
            Back To Account
          </Link>
        </Button>
        
      </div>
     
      

    </>
  )

}