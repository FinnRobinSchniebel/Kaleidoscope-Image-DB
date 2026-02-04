'use client'

import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible'
import { ScrollArea, ScrollBar } from '@/components/ui/scroll-area'
import { ChevronLeft, CornerDownRight, CornerLeftUp, FolderArchive } from 'lucide-react'
import Link from 'next/link'
import React, { useActionState, useCallback, useEffect, useState } from 'react'
import FolderList from './FolderList'
import UploadInstructions from './UploadInstructions'
import DropFile from './DropFile'
import { SubmitErrorHandler, SubmitHandler, useForm } from 'react-hook-form'

export type UploadFormValues = {
  zipFile: File | null
  structureZip: string
  folders: string[]
  files: string
}
type Props = {}

export default function AccountLayout({ }: Props) {

  const MAX_FILE_SIZE_MB = 5120



  const [Layers, setLayers] = useState(0)

  const [validFile, setValidFile] = useState(false)

  const { register, control, handleSubmit, setValue, watch , formState:{errors}} = useForm<UploadFormValues>({
    defaultValues: {
      zipFile: null,
      structureZip: "",
      folders: Array(Layers).fill("fff"),
      files: "[Order]",
    },
  })

  useEffect(() => { console.log(Layers) }, [Layers])

  
  const onSubmit: SubmitHandler<UploadFormValues> = (data: UploadFormValues) => {
    if (!data.zipFile) {
      return
    }

    const formData = new FormData()
    formData.append("zipFile", data.zipFile)
    formData.append("structureZip", data.structureZip)
    formData.append("files", data.files)

    data.folders.forEach((folder, i) => {
      formData.append(`folders[${i}]`, folder)
    })

    console.log(formData)
  }

 const onError = (errors: any, event?: React.BaseSyntheticEvent) => {
    console.log(errors); // The validation errors
    alert("What on earth did you do to get this error. This thing is type safe!")
  };


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

        <UploadInstructions MaxSize={MAX_FILE_SIZE_MB} />

        <form onSubmit={handleSubmit(onSubmit, onError)} className='grid grid-cols-1 gap-2 auto-rows-auto justify-items-start p-10 m-2 rounded-2xl bg-accent w-[90%] xl:w-[60%] place-self-center'>
          <DropFile FormRegister={register} SetFormValue={setValue} MaxSize={MAX_FILE_SIZE_MB} 
          ValidFile={(e) => { setValidFile(e || watch("zipFile") != null)}/*There is an assumption that if an error occurs any valid existing file will stay*/} 
          />
          <ScrollArea aria-orientation='horizontal' className='w-full overflow-x-auto pb-5'>
            <FolderList FormRegister={register} Layers={Layers} />
            <ScrollBar orientation='horizontal' className='h-3' />
          </ScrollArea>

          <div className='justify-self-center'>
            <Button type='button' className='mx-2 w-fit justify-self-center bg-green-600/80 cursor-pointer' onClick={() => { setLayers((e) => { return Math.min(e + 1, 10) }) }}>
              <CornerDownRight className=' size-[1rem]' />
              Add Folder Level
            </Button>

            <Button type='button' className='mx-2 w-fit bg-red-800/80 cursor-pointer' onClick={() => { setLayers((e) => { return Math.max(e - 1, 0) }) }}>
              <CornerLeftUp className=' size-[1rem]' />
              Remove Folder Level
            </Button>
          </div>

          <div className=' justify-self-center bg-accent w-full rounded-xl p-5 px-10'>
            <Button type='submit' disabled={!validFile} className=' bg-green-700 cursor-pointer px-10 py-4 rounded-2xl'>Submit</Button>
          </div>
        </form>

      </div >
    </>
  )
}
