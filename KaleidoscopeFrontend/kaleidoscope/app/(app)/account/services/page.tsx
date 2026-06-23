'use client'

import { MenuButtonProps } from "@/components/KscopeSharedUI/account/IconButtonsMenu";
import MenuButtons from "@/components/KscopeSharedUI/account/MenuButtons";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { ChevronLeft } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import ServiceDialog, { ServiceDialogOptions } from './serviceDialog'



interface Props {

}


export default function page({ }: Props) {

  const [dialogOpen, setDialogOpen] = useState(false)


  const ServiceInfo = [
    {ServiceName: "Pixiv/Fanbox", BackendName: "Pixiv/Fanbox", fields: {apiKey:"Access Token", apiKey2:"Refresh Token"}, Warnings: "test", Info: "Provide a set of keys to use for accessing your accout."} satisfies ServiceDialogOptions
  ]


  const Buttons = [
    { icon: "/PixivIcon.webp", label: "Pixiv", loc: "", func: () => { setDialogOpen(true) } } satisfies MenuButtonProps,

  ]

  return (
    <>

      <h1 className='p-10 text-4xl'>Connected Services</h1>

      <div className='flex flex-col flex-1 w-full'>

        <Button className='m-4 w-fit bg-accent shadow-primary/60 hover:bg-accent/30' variant='outline' asChild>
          <Link href={`/account`}>
            <ChevronLeft></ChevronLeft>
            Back To Account
          </Link>
        </Button>
        <div className='grid grid-cols-2 w-full py-20 gap-4 p-4'>
          <MenuButtons Buttons={Buttons} />
        </div>
      </div>
      <ServiceDialog currentOpenState={dialogOpen} dialog={ServiceInfo[0]} changeOpen={setDialogOpen}/>
    </>

  )
}