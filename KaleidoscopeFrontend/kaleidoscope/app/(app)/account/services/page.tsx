'use client'

import { MenuButtonProps } from "@/components/KscopeSharedUI/account/IconButtonsMenu";
import MenuButtons from "@/components/KscopeSharedUI/account/MenuButtons";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { ChevronLeft } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import ConnectPixiv from '@/app/(app)/account/services/connectPixiv'
import ServiceDialog, { ServiceDialogOptions } from './serviceDialog'



interface Props {

}


export default function page({ }: Props) {

  const [dialogOpen, setDialogOpen] = useState(false)
  const [serviceIndex, setServiceIndex] = useState(0)


  const ServiceInfo = [
    { ServiceName: "Pixiv/Fanbox", BackendName: "pixiv", fields: { userName: "User ID", apiKey: "Pixiv APP API Token" }, Info: "Manual: Provide your Pixiv user ID (number in the url when you access your Pixiv profile AND a valid APP refresh token retrieved form an external tool." } satisfies ServiceDialogOptions
  ]


  const Buttons = [
    { icon: "/PixivIcon.webp", label: "Pixiv", loc: "", func: (i : number) => { setDialogOpen(true); setServiceIndex(i) } } satisfies MenuButtonProps,

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


      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className='overflow-y-auto max-h-[90vh] bg-background/80 text-primary rounded-2 min-w-1/4 min-h-3/4 block' OverlayClassName='bg-black/40 backdrop-blur-[2px]'>
          <DialogHeader className='mb-1 mt-8 '>
            <DialogTitle className='font-bold text-3xl text-center'> {ServiceInfo[serviceIndex].ServiceName} </DialogTitle>
          </DialogHeader>
          <DialogDescription className='text-primary size-fit my-2'>
            Connect a service to your Kaleidoscope Library
          </DialogDescription>
          {serviceIndex == 0 && (
            <>
            <ConnectPixiv onOpenChange={setDialogOpen}></ConnectPixiv>

            <h1 className="font-bold text-center m-2  text-2xl">
              OR
            </h1>
            </>
          )          
          }
            <ServiceDialog dialog={ServiceInfo[serviceIndex]} changeOpen={setDialogOpen} currentOpenState={dialogOpen}></ServiceDialog>
        </DialogContent>
      </Dialog >
    </>
  )

}