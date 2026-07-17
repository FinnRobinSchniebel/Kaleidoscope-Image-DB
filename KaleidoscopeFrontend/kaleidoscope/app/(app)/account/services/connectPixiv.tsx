'use client'

import SeparatorBorder from '@/components/KscopeSharedUI/SeparatorBorder'
import { GetServiceCredentials } from '@/components/api/GetServiceCredentials-api'
import { GORequest } from '@/components/api/apicaller'
import { useProtected } from '@/components/api/jwt_apis/ProtectedProvider'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Field, FieldContent, FieldDescription, FieldLabel, FieldTitle } from '@/components/ui/field'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import React, { Dispatch, SetStateAction, useEffect, useState } from 'react'

const PIXIV_LOGIN_URL = 'https://app-api.pixiv.net/web/v1/login'

async function generatePKCE(): Promise<{ codeVerifier: string; codeChallenge: string }> {
  const bytes = crypto.getRandomValues(new Uint8Array(32))
  const codeVerifier = btoa(String.fromCharCode(...bytes))
    .replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')

  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(codeVerifier))
  const codeChallenge = btoa(String.fromCharCode(...new Uint8Array(digest)))
    .replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')

  return { codeVerifier, codeChallenge }
}

interface Props {

  onOpenChange: Dispatch<SetStateAction<boolean>>
}

export default function ConnectPixiv({ onOpenChange }: Props) {
  const protectedApi = useProtected()

  const [codeVerifier, setCodeVerifier] = useState('')
  const [code, setCode] = useState('')
  const [pixivUserId, setPixivUserId] = useState('')
  const [syncInterval, setSyncInterval] = useState('0')
  const [loginStarted, setLoginStarted] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) {
      setCode('')
      setCodeVerifier('')
      setLoginStarted(false)
      setError('')
      return
    }
    GetServiceCredentials('pixiv', protectedApi).then(creds => {
      if (!creds) return
      setPixivUserId(creds.username ?? '')
      setSyncInterval(String(creds.sync_interval_hours ?? 0))
    })
  }, [open])

  async function handleLogin() {
    const { codeVerifier, codeChallenge } = await generatePKCE()
    setCodeVerifier(codeVerifier)
    const params = new URLSearchParams({
      code_challenge: codeChallenge,
      code_challenge_method: 'S256',
      client: 'pixiv-android',
    })
    window.open(`${PIXIV_LOGIN_URL}?${params}`, '_blank')
    setLoginStarted(true)
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')

    const formData = new FormData()
    formData.append('code', code)
    formData.append('code_verifier', codeVerifier)
    formData.append('pixiv_user_id', pixivUserId)
    formData.append('sync_interval_hours', syncInterval)

    const request: GORequest = {
      endpoint: '/service/pixivconnect',
      type: 'POST',
      header: {},
      formData,
    }
    const { status } = await protectedApi.CallProtectedAPI(request)
    if (status === 200) {
      onOpenChange(false)
    } else {
      setError('Connection failed. Check your code and try again.')
    }
  }

  const canSubmit = loginStarted && code && pixivUserId

  return (

    <SeparatorBorder>
      <form onSubmit={handleSubmit} className='flex flex-col gap-4'>

        <div className='flex flex-col gap-2'>
          <label className='font-bold'>Step 1 — Authorize</label>
          <button
            type='button'
            onClick={handleLogin}
            className='bg-blue-500/40 border-1 border-blue-500 p-2 rounded font-bold hover:shadow-sm active:bg-accent'
          >
            {loginStarted ? 'Re-open Pixiv Login' : 'Login to Pixiv'}
          </button>
          {loginStarted && (
            <p className='text-sm text-muted-foreground'>
              After logging in, copy the <code>code=</code> value from the redirect URL and paste it below.
            </p>
          )}
        </div>


        <div>
          <label className='font-bold'>Step 2 — Paste Authorization Code
          <p className='text-muted-foreground font-normal text-sm'>After login, you will come to a white page. Right click the screen, and click "Inspect".
            In the Inspect menu on the top bar, click "Console". Copy the code from the URL inside the "Failed to launch" error.</p>
          </label>
          <input
            required
            value={code}
            onChange={e => setCode(e.target.value)}
            placeholder='Paste the code= value from the redirect URL'
            className='w-full border border-primary/60 p-2 rounded mt-1'
          />
        </div>


        <div>
          <label className='font-bold'>Pixiv User ID</label>
          <input
            required
            value={pixivUserId}
            onChange={e => setPixivUserId(e.target.value)}
            placeholder='Your numeric Pixiv user ID'
            className='w-full border border-primary/60 p-2 rounded mt-1'
          />
        </div>

        <div>
          <label className='font-bold'>Sync Frequency</label>
          <RadioGroup
            value={syncInterval}
            onValueChange={setSyncInterval}
            className='flex flex-row gap-0'
            name='sync_interval_hours'
          >
            <FieldLabel htmlFor='pv-none' className='border-primary/60 has-data-[state=checked]:bg-primary-foreground'>
              <Field orientation='horizontal'>
                <FieldContent>
                  <FieldTitle>None</FieldTitle>
                  <FieldDescription>No automated syncing</FieldDescription>
                </FieldContent>
                <RadioGroupItem value='0' id='pv-none' className='hidden' />
              </Field>
            </FieldLabel>
            <FieldLabel htmlFor='pv-day' className='border-primary/60 has-data-[state=checked]:bg-primary-foreground'>
              <Field orientation='horizontal'>
                <FieldContent>
                  <FieldTitle>Daily</FieldTitle>
                  <FieldDescription>Every 24 hours</FieldDescription>
                </FieldContent>
                <RadioGroupItem value='24' id='pv-day' className='hidden' />
              </Field>
            </FieldLabel>
            <FieldLabel htmlFor='pv-week' className='border-primary/60 has-data-[state=checked]:bg-primary-foreground'>
              <Field orientation='horizontal'>
                <FieldContent>
                  <FieldTitle>Weekly</FieldTitle>
                  <FieldDescription>Every 7 days</FieldDescription>
                </FieldContent>
                <RadioGroupItem value='168' id='pv-week' className='hidden' />
              </Field>
            </FieldLabel>
          </RadioGroup>
        </div>

        {error && <p className='text-destructive text-sm'>{error}</p>}

        <button
          type='submit'
          className={`mt-4 border-1 border-gray-500 p-2 rounded font-bold hover:shadow-sm active:bg-accent transition-colors ${canSubmit ? 'bg-green-600/60' : 'bg-primary/10'
            }`}
        >
          Connect
        </button>
      </form>
    </SeparatorBorder>
  )
}
