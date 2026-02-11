'use client'

// Below is info for Comments for important logic, check the number on the comment to find more info.

// Button Checkbox Related Logic:
//  1. Increase
//  2. Decrease
//  3. EdgeCase
// 4. input array length info
// 5. Backend submission empty fields info

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
import PostZip from '@/components/api/PostZip-api'
import { useProtected } from '@/components/api/jwt_apis/ProtectedProvider'
import { Item } from '@radix-ui/react-navigation-menu'

export type UploadFormValues = {
  zipFile: File | null
  structureZip: string
  folders: string[]
  files: string
  GroupingLevel: string
}
type Props = {}

export default function page({ }: Props) {

  const MAX_FILE_SIZE_MB = 5120

  const [errorMessage, setErrorMessage] = useState("")
  const [SuccessMessage, setSuccessMessage] = useState(false)

  const [Layers, setLayers] = useState(0)

  const [validFile, setValidFile] = useState(false)

  const { register, control, handleSubmit, setValue, watch, formState: { errors } } = useForm<UploadFormValues>({
    defaultValues: {
      zipFile: null,
      structureZip: "",
      //4. warning: this array does not decrease as layers are removed. It maintains the longest number of inputs used since Load. Use 'Layers' for correct length
      folders: Array(Layers).fill(""),
      files: "[Order]",
      GroupingLevel: "1" //upto Layers (Max 11)
    },
  })

  //3. Effect is used after decrementing to make sure the value does not fall out of bounds when decrementing. 
  // Must be a effect because doing this in a button would result in a desync of the visual checkbox (the original update has to have taken place)
  useEffect(() => {
    const current = Number(watch("GroupingLevel") ?? 0)
    const maxAllowed = Layers + 1

    if (current > maxAllowed) {
      setValue("GroupingLevel", String(maxAllowed), {
        shouldDirty: true,
        shouldTouch: true,
      })
    }
    

  }, [Layers])

  const protectedApi = useProtected()

  const onSubmit: SubmitHandler<UploadFormValues> = async (data: UploadFormValues) => {
    if (!data.zipFile) {
      return
    }

    const formData = new FormData()
    formData.append("zipFile", data.zipFile)
    formData.append("structureZip", data.structureZip)
    formData.append("files", data.files)
    formData.append("GroupingLevel", `${data.GroupingLevel}`)

    setErrorMessage("")
    setSuccessMessage(false)

    //5. FormData does not decude empty strings sent from front end into empty fields. If a level has no data 'NAN' is added so its not ignored.
    data.folders.forEach((folder, i) => {
      if (i >= Layers) {
        return
      }
      formData.append(`folders`, folder == "" ? "NAN" : folder)
    })

    const { status, response, errorString } = await PostZip({ form: formData, protectedApi: protectedApi })

    if (errorString != undefined) {
      setErrorMessage(errorString)
    }

    if (response != undefined || (status >= 200 && status <= 299)) {
      setSuccessMessage(true)
    }

  }

  const onError = (errors: any, event?: React.BaseSyntheticEvent) => {
    console.log(errors); // The validation errors
    alert("What on earth did you do to get this error. This thing is type safe!")
  };

  return (
    <>
      <h1 className='p-10 text-4xl'>Upload Files</h1>

      <div className='flex flex-col flex-1 w-full'>

        <Button className='m-4 w-fit bg-accent ' variant='outline' asChild>
          <Link href={`/account`}>
            <ChevronLeft></ChevronLeft>
            Back To Account
          </Link>
        </Button>
        {/* Instructions Box */}
        <UploadInstructions MaxSize={MAX_FILE_SIZE_MB} />

        <form onSubmit={handleSubmit(onSubmit, onError)} className='grid grid-cols-1 gap-2 auto-rows-auto justify-items-start p-10 m-2 rounded-2xl bg-accent w-[90%] xl:w-[60%] place-self-center'>

          {/* Drop File box */}
          <DropFile FormRegister={register} SetFormValue={setValue} MaxSize={MAX_FILE_SIZE_MB} watch={watch}
            ValidFile={(e) => { setValidFile(e) }/*There is an assumption that if an error occurs any valid existing file will stay*/}
          />

          {/* Folder Level Parsing Inputs */}
          <ScrollArea aria-orientation='horizontal' className='w-full overflow-x-auto pb-5'>
            <FolderList FormRegister={register} Layers={Layers} />
            <ScrollBar orientation='horizontal' className='h-3' />
          </ScrollArea>

          {/* Increase/Decrease buttons for folder levels */}
          <div className='justify-self-center'>
            <Button type='button' className='mx-2 w-fit justify-self-center bg-green-600/80 cursor-pointer' onClick={() => {
              const groupingLevel = watch("GroupingLevel")
              const current = Number(groupingLevel ?? 0)
              const FolderCount = watch("folders").length
              setLayers((e) => {
                //1. If the current selected is the file slot it needs to increment by one to not fall to the folder level
                //+1 because files length starts at 1 not at 0
                if (current == e + 1) {
                  setValue("GroupingLevel", String(current + 1), {
                    shouldDirty: true,
                    shouldTouch: true,
                  })
                }


                return Math.min(e + 1, 10)
              })
            }}>
              <CornerDownRight className=' size-[1rem]' />
              Add Folder Level
            </Button>

            <Button type='button' className='mx-2 w-fit bg-red-800/80 cursor-pointer'
              onClick={() => {
                const groupingLevel = watch("GroupingLevel")
                const current = Number(groupingLevel ?? 0)
                setLayers((e) => {
                  if (e == 0) {
                    return e
                  }
                  //2. If the current checked is the last in the list of 'Files' then it needs to be pushed down to the next highest when decrementing 
                  if (current == e) {
                    setValue("GroupingLevel", String(current - 1), {
                      shouldDirty: true,
                      shouldTouch: true,
                    })
                  }
                  return Math.max(e - 1)
                })
              }}>
              <CornerLeftUp className=' size-[1rem]' />
              Remove Folder Level
            </Button>
          </div>

          <div className=' justify-self-center bg-accent w-full rounded-xl p-5 px-10 text-primary-foreground'>
            <Button type='submit' disabled={!validFile} className=' bg-green-700 cursor-pointer px-10 py-4 rounded-2xl'>Submit</Button>

            <div className={`bg-radial-[at_-10%_20%] from-green-400/80 to-70% to-green-700/70 rounded-lg m-2 w-full lg:w-4/5 justify-self-center 
              transition-[max-height] duration-300 ease-out relative overflow-hidden 
              ${SuccessMessage ? 'max-h-[200px] min-h-20 lg:min-h-0' : 'max-h-0 '}`}>

              <button type='button' onClick={() => { setSuccessMessage(false) }} className='absolute right-5 top-1 lg:top-2 border-2 px-2 min-h-6 rounded-md hover:bg-accent cursor-pointer'>x</button>
              <div className='p-5'>
                Successfully added!
              </div>
            </div>
            
            <div
              className={`
                bg-radial-[at_-10%_20%] from-red-400/80 to-70% to-red-800/70 rounded-lg w-full lg:w-4/5 justify-self-center
                relative overflow-hidden
                transition-[max-height] duration-300 ease-out
                ${errorMessage !="" ? 'max-h-[200px] m-2 min-h-20 lg:min-h-0' : 'max-h-0 '}
              `}
            >
              <div className="p-5">
                <button type="button" onClick={() => setErrorMessage("")}
                  className="absolute right-5 top-2 border-2 px-2 min-h-6 rounded-md hover:bg-accent"
                >
                  x
                </button>

                Error: {errorMessage}
              </div>
            </div>
          </div>

        </form>



      </div >
    </>
  )
}
