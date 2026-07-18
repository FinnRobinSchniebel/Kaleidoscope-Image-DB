
import connectExteranel_api from '@/components/api/connectExternal-api'
import { getServiceCredentials } from '@/components/api/getServiceCredentials-api'
import { useProtected } from '@/components/api/jwt_apis/ProtectedProvider'
import SeparatorBorder from '@/components/KscopeSharedUI/SeparatorBorder'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Field, FieldContent, FieldDescription, FieldLabel, FieldTitle } from '@/components/ui/field'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import React, { Dispatch, SetStateAction, useEffect, useRef, useState } from 'react'
import { ServiceDialogOptions } from './serviceDialog'


interface Props {
  changeOpen: Dispatch<SetStateAction<boolean>>
  currentOpenState: boolean
  dialog: ServiceDialogOptions
}


type FieldValues = {
  username: string
  password: string
  apiKey1: string
  apiKey2: string
  syncIntervalHours: string
}

const emptyFields: FieldValues = {
  username: '',
  password: '',
  apiKey1: '',
  apiKey2: '',
  syncIntervalHours: '0',
}

export default function ConnectService({ currentOpenState, changeOpen, dialog }: Props) {

  const protectedApi = useProtected()
  const formRef = useRef<HTMLFormElement>(null)
  const [fields, setFields] = useState<FieldValues>(emptyFields)

  useEffect(() => {
    if (!currentOpenState) {
      setFields(emptyFields)
      return
    }
    getServiceCredentials(dialog.BackendName, protectedApi).then(creds => {
      if (!creds) return
      setFields({
        username: creds.username ?? '',
        password: creds.password ?? '',
        apiKey1: creds.key1 ?? '',
        apiKey2: creds.key2 ?? '',
        syncIntervalHours: String(creds.sync_interval_hours ?? 0),
      })
    })
  }, [currentOpenState])

  function set(field: keyof FieldValues) {
    return (e: React.ChangeEvent<HTMLInputElement>) =>
      setFields(prev => ({ ...prev, [field]: e.target.value }))
  }

  async function handleSubmit(e: React.FormEvent) {
    if (!formRef.current) return

    e.preventDefault()

    const formData = new FormData(formRef.current)

    await connectExteranel_api(dialog.BackendName, formData, protectedApi)

  }

  return (
    <SeparatorBorder>
      <div className='text-left font-bold'>
        {dialog.Info ?? "Connect a service"}
      </div>

      {
        dialog.Warnings && (
          <div className='border-yellow-300/80 bg-yellow-50 border-3 rounded-sm m-2 p-2 text-yellow-600 mb-4'>
            {dialog.Warnings}
          </div>
        )
      }

      <form ref={formRef} onSubmit={handleSubmit} className="group flex flex-col gap-4">

        {/* Username */}
        {dialog.fields.userName && (
          <div>
            <label>{dialog.fields.userName}</label>
            <input
              required
              value={fields.username}
              onChange={set('username')}
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
              required
              type="password"
              value={fields.password}
              onChange={set('password')}
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
              required
              value={fields.apiKey1}
              onChange={set('apiKey1')}
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
              required
              value={fields.apiKey2}
              onChange={set('apiKey2')}
              className="w-full border border-primary/60 p-2 rounded"
              name='apiKey2'
            />
          </div>
        )}

        <div>
          <label className='font-bold'>Sync Frequency</label>
          <RadioGroup
            value={fields.syncIntervalHours}
            onValueChange={v => setFields(prev => ({ ...prev, syncIntervalHours: v }))}
            className='flex flex-row gap-0'
            name="sync_interval_hours"
          >
            <FieldLabel htmlFor="none" className=' border-primary/60 has-data-[state=checked]:bg-primary-foreground'>
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
            <FieldLabel htmlFor="day" className=' border-primary/60 has-data-[state=checked]:bg-primary-foreground'>
              <Field orientation="horizontal">
                <FieldContent>
                  <FieldTitle>Daily</FieldTitle>
                  <FieldDescription>
                    Every 24 hours
                  </FieldDescription>
                </FieldContent>
                <RadioGroupItem value="24" id="day" className='hidden' />
              </Field>
            </FieldLabel>
            <FieldLabel htmlFor="week" className=' border-primary/60 has-data-[state=checked]:bg-primary-foreground'>
              <Field orientation="horizontal">
                <FieldContent>
                  <FieldTitle>Weekly</FieldTitle>
                  <FieldDescription>
                  Every 7 days
                  </FieldDescription>
                </FieldContent>
                <RadioGroupItem value="168" id="week" className='hidden' />
              </Field>
            </FieldLabel>
          </RadioGroup>
        </div>

        <button
          type="submit"
          className="mt-4 bg-green-600/60 group-has-[input:invalid]:bg-primary/10 border-1 border-gray-500 shadow-black shadow-xs [input:invalid]:shadow-none p-2 rounded font-bold
             hover:shadow-sm
             active:bg-accent transition-colors"
        >
          Connect
        </button>

      </form>
    </SeparatorBorder>
  )
}
