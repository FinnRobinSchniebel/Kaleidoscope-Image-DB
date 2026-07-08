
import ConnectExteranel_api from '@/components/api/ConnectExternal-api'
import { useProtected } from '@/components/api/jwt_apis/ProtectedProvider'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Field, FieldContent, FieldDescription, FieldLabel, FieldTitle } from '@/components/ui/field'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import React, { Dispatch, SetStateAction, useRef, useState } from 'react'


interface Props {
  changeOpen: Dispatch<SetStateAction<boolean>>
  currentOpenState: boolean
  dialog: ServiceDialogOptions
}

export type ServiceDialogOptions = {
  ServiceName: string
  BackendName: string
  Info?: string
  fields: {
    userName?: string
    password?: string
    apiKey?: string
    apiKey2?: string
    SyncIntervalHours?: number
  }
  Warnings?: string

}

export default function ServiceDialog({ currentOpenState, changeOpen, dialog }: Props) {


  const protectedApi = useProtected()
  const formRef = useRef<HTMLFormElement>(null)


  async function handleSubmit(e: React.FormEvent) {
    if (!formRef.current) return

    e.preventDefault()

    const formData = new FormData(formRef.current)
    formData.append("service", dialog.BackendName)

    await ConnectExteranel_api(formData, protectedApi)

  }

  return (
    <Dialog open={currentOpenState} onOpenChange={changeOpen}>
      <DialogContent className='overflow-hidden bg-background/80 text-primary rounded-2 min-w-1/4 min-h-3/4 block' OverlayClassName='bg-black/40 backdrop-blur-[2px]'>
        <DialogHeader className='mb-1 mt-8 '>
          <DialogTitle className='font-bold text-3xl text-center'> {dialog.ServiceName} </DialogTitle>
        </DialogHeader>
        <DialogDescription className='text-primary size-fit my-2'>
          Connect a service to your Kaleidoscope Library
        </DialogDescription>
        <div className='text-left font-bold'>
          {dialog.Info ?? "Connect a service"}
        </div>

        {dialog.Warnings && (
          <div className='border-yellow-300/80 bg-yellow-50 border-3 rounded-sm m-2 p-2 text-yellow-600 mb-4'>
            {dialog.Warnings}
          </div>
        )}

        <form ref={formRef} onSubmit={handleSubmit} className="flex flex-col gap-4">

          {/* Username */}
          {dialog.fields.userName && (
            <div>
              <label>{dialog.fields.userName}</label>
              <input
                className="w-full border border-primary/60 p-2 rounded"
                name='username'
              />
            </div>
          )}

          {/* Password */}
          {dialog.fields.password && (
            <div>
              <label className='font-bold'>{dialog.fields.password}</label>
              <input
                type="password"
                className="w-full border border-primary/60 p-2 rounded"
                name='password'
              />
            </div>
          )}

          {/* API Key 1 */}
          {dialog.fields.apiKey && (
            <div>
              <label className='font-bold'>{dialog.fields.apiKey}</label>
              <input
                className="w-full  border border-primary/60 p-2 rounded"
                name='apiKey1'
              />
            </div>
          )}

          {/* API Key 2 */}
          {dialog.fields.apiKey2 && (
            <div>
              <label className='font-bold'>{dialog.fields.apiKey2}</label>
              <input
                className="w-full border border-primary/60 p-2 rounded"
                name='apiKey2'
              />
            </div>
          )}

          <div>
            <label className='font-bold'>Sync Frequency</label>
            <RadioGroup defaultValue='0' className='flex flex-row gap-0' name="syncIntervalHours">
              <FieldLabel htmlFor="none"  className=' border-primary/60 has-data-[state=checked]:bg-primary-foreground'>
                <Field orientation="horizontal" >
                  <FieldContent>
                    <FieldTitle>None</FieldTitle>
                    <FieldDescription>
                      No automated syncing
                    </FieldDescription>
                  </FieldContent>
                  <RadioGroupItem value="0" id="none" className='hidden' />
                </Field>
              </FieldLabel>
              <FieldLabel htmlFor="day"  className=' border-primary/60 has-data-[state=checked]:bg-primary-foreground'>
                <Field orientation="horizontal">
                  <FieldContent>
                    <FieldTitle>Daily</FieldTitle>
                    <FieldDescription>
                      Perform a sync every 24 hours
                    </FieldDescription>
                  </FieldContent>
                  <RadioGroupItem value="24" id="day" className='hidden'/>
                </Field>
              </FieldLabel>
              <FieldLabel htmlFor="week"  className=' border-primary/60 has-data-[state=checked]:bg-primary-foreground'>
                <Field orientation="horizontal">
                  <FieldContent>
                    <FieldTitle>Weekly</FieldTitle>
                    <FieldDescription>
                      Performs a sync every 7 days
                    </FieldDescription>
                  </FieldContent>
                  <RadioGroupItem value="168" id="week" className='hidden'/>
                </Field>
              </FieldLabel>
            </RadioGroup>
          </div>

          <button
            type="submit"
            className="mt-4 bg-primary/10 border-1 border-gray-500 shadow-black p-2 rounded font-bold
             hover:shadow-sm 
             active:bg-accent"
          >
            Connect
          </button>
        </form>
      </DialogContent>
    </Dialog >
  )
}
