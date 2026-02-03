'use client'

import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible'
import { ScrollArea, ScrollBar } from '@/components/ui/scroll-area'
import { ChevronLeft, CornerDownRight, CornerLeftUp, FolderArchive } from 'lucide-react'
import Link from 'next/link'
import React, { useActionState, useEffect, useState } from 'react'



type Props = {}

export default function AccountLayout({ }: Props) {

  const MaxFileSize = "5 GB"
  const [instructionsOpen, setInstructionsOpen] = useState(false)

  const [data, action, isPending] = useActionState(SendFile, undefined)

  const [Layers, setLayers] = useState(2)
  useEffect(() => { console.log(Layers) }, [Layers])

  return (
    <>
      <div className='p-10 text-4xl'>Upload Files</div>


      <div className='flex flex-col flex-1 w-full'>

        <Button className='m-4 w-fit bg-accent ' variant='outline' asChild>
          <Link href={`/account`}>
            <ChevronLeft></ChevronLeft>
            Back To Account
          </Link>
        </Button>

        <Collapsible className={`bg-accent xl:w-[60%] place-self-center  rounded-2xl py-2 ${instructionsOpen ? "" : "cursor-pointer"}`}
          open={instructionsOpen}
          onOpenChange={setInstructionsOpen}
          onClick={() => { if (!instructionsOpen) { setInstructionsOpen(true) } }}
        >
          <h2 className='text-2xl'>Instructions</h2>

          {!instructionsOpen && <p className='font-normal'>Click to show instructions</p>}
          <CollapsibleContent className='font-normal py-2 flex flex-col p-10'>
            <div className='justify-start text-start '>
              <ol type='1' className='list-decimal'>
                <li>
                  Upload a zip file with all the images you want to add.
                  <ol className='list-disc list-outside ml-5 mt-2'>
                    <li className=''> The file contents should be organized into folders and files with meaningful names.</li>
                    <li className=''> Kaleidoscope will group files by source ID or folder (depending on your choice).</li>
                    <li className=''> The File cannot be larger than {MaxFileSize}.</li>
                  </ol>
                </li>
                <li>
                  Using the options below the update describe the file formatting.
                  Each level indicates the naming convention used by your folders.
                  Adding levels will increase the depth of your folder structure with
                  the lowest one always being the naming convention of the image files.
                  <ol className='list-disc list-outside ml-5 mt-2'>
                    <li className=''> Names for all Files:</li>
                      <ol>
                        <li className="relative pl-4 before:absolute before:left-0 before:content-['-']">
                          '[Author]' : Authors name
                        </li>
                        <li className="relative pl-4 before:absolute before:left-0 before:content-['-']">
                          '[ID]' : Source ID
                        </li>
                        <li className="relative pl-4 before:absolute before:left-0 before:content-['-']">
                          '[Source]' : Source name (ex: Pixiv, Twitter, Instagram)
                        </li>
                        <li className="relative pl-4 before:absolute before:left-0 before:content-['-']">
                          '[AuthorId]' : author's ID (if the source give the author an ID)
                        </li>
                        <li className="relative pl-4 before:absolute before:left-0 before:content-['-']">
                          '[Title]' : Image Set Title
                        </li>
                        <li className="relative pl-4 before:absolute before:left-0 before:content-['-']">
                          '[Date]' : Date posted (02_02_2026)
                        </li>
                      </ol>
                    <li className=''> For Files only: </li>
                      <ol>
                        <li className="relative pl-4 before:absolute before:left-0 before:content-['-']">
                          '[Order]' : Part of File Name used for determining the order. Will use alphabetical/numerical (a before b and 0 before 1)
                          '[-Order]' : Part of File Name used for determining the order. Will use reverse alphabetical/numerical (b before a and 1 before 0)
                        </li>
                        
                      </ol>
                  </ol>
                </li>
              </ol>

            </div>

            <Button onClick={() => setInstructionsOpen(false)} className='max-w-40 size-fit cursor-pointer mt-2'>close</Button>
          </CollapsibleContent>
        </Collapsible>

        <form action={action} className='grid grid-cols-1 gap-2 auto-rows-auto justify-items-start p-10 m-2 rounded-2xl bg-accent xl:w-[60%] place-self-center'>
          <ScrollArea aria-orientation='horizontal' className='w-full overflow-x-auto pb-5'>
            {Array.from({ length: Layers }, (_, index) =>
            (
              <div key={index} className={`flex min-w-0 mx-1 items-center  ml-[calc(var(--i)*2rem)]`} style={{ "--i": index } as React.CSSProperties}>

                <label htmlFor={`folder-${index}`} className='text-justify flex'>
                  {index == 0 && <FolderArchive className='p-2 size-10' />}
                  {index != 0 && <CornerDownRight className='p-2 size-10' />}
                  <span className="self-center-safe text-ellipsis overflow-hidden whitespace-nowrap">
                    {index == 0 ? "Zip File" : (index === Layers - 1 ? "File(s)" : "Folder")}
                  </span>
                </label>

                <input id={`folder-${index}`} className='outline-3 field-sizing-content outline-accent rounded-sm px-5 mx-3 h-8 min-w-30 max-w-90 '></input>
              </div>
            ))}
            <ScrollBar orientation='horizontal' className='h-3' />
          </ScrollArea>
        </form>
        <div className=''>
          <Button className='mx-2 w-fit justify-self-center bg-green-600/80 cursor-pointer' onClick={() => { setLayers((e) => { return Math.min(e + 1, 10) }) }}>
            <CornerDownRight className=' size-[1rem]' />
            Add Folder Level
          </Button>

          <Button className='mx-2 w-fit bg-red-800/80 cursor-pointer' onClick={() => { setLayers((e) => { return Math.max(e - 1, 2) }) }}>
            <CornerLeftUp className=' size-[1rem]' />
            Remove Folder Level
          </Button>
        </div>



      </div >
    </>
  )
}

async function SendFile(previousState: unknown, formData: FormData) {

}