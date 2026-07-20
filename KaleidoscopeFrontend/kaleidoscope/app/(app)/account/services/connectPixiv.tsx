'use client'

import SeparatorBorder from '@/components/KscopeSharedUI/SeparatorBorder'
import { getServiceCredentials } from '@/components/api/getServiceCredentials-api'
import { GORequest } from '@/components/api/apicaller'
import { useProtected } from '@/components/api/jwt_apis/ProtectedProvider'
import { Field, FieldContent, FieldDescription, FieldLabel, FieldTitle } from '@/components/ui/field'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import React, { Dispatch, SetStateAction, useEffect, useRef, useState } from 'react'

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
  currentOpenState: boolean
}

export default function ConnectPixiv({ onOpenChange, currentOpenState }: Props) {
  const protectedApi = useProtected()
  const formRef = useRef<HTMLFormElement>(null)

  const [codeVerifier, setCodeVerifier] = useState('')
  const [code, setCode] = useState('')
  const [pixivUserId, setPixivUserId] = useState('')
  const [loginStarted, setLoginStarted] = useState(false)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!currentOpenState) {
      setCode('')
      setCodeVerifier('')
      setLoginStarted(false)
      setError('')
      return
    }
    getServiceCredentials('pixiv', protectedApi).then(creds => {
      if (!creds) return
      setPixivUserId(creds.username ?? '')
    })
  }, [currentOpenState])

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
    setIsSubmitting(true)
    const formData = new FormData(formRef.current!)
    formData.append('code_verifier', codeVerifier)
    const { status } = await protectedApi.CallProtectedAPI({
      endpoint: '/service/pixivconnect',
      type: 'POST',
      header: {},
      formData,
    } satisfies GORequest)
    setIsSubmitting(false)
    if (status === 200) {
      onOpenChange(false)
    } else {
      setError('Connection failed. Check your code and try again.')
    }
  }

  return (
    <SeparatorBorder>
      <form ref={formRef} onSubmit={handleSubmit} className='group flex flex-col gap-4'>

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
          <label className='font-bold'>
            Step 2 — Paste Authorization Code
            <p className='text-muted-foreground font-normal text-sm'>
              After login, you will come to a white page. Right click the screen, and click "Inspect".
              In the Inspect menu on the top bar, click "Console". Copy the code from the URL inside the "Failed to launch" error.
            </p>
          </label>
          <input
            required
            name='code'
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
            name='pixiv_user_id'
            value={pixivUserId}
            onChange={e => setPixivUserId(e.target.value)}
            placeholder='Your numeric Pixiv user ID'
            className='w-full border border-primary/60 p-2 rounded mt-1'
          />
        </div>

       

        {error && <p className='text-destructive text-sm'>{error}</p>}

        <button
          type='submit'
          disabled={isSubmitting}
          className='mt-4 bg-green-200/20 border-1 border-green-800/80 shadow-black shadow-xs p-2 rounded font-bold hover:shadow-sm active:bg-accent transition-colors  
          group-has-[input:invalid]:hover:shadow-xs  group-has-[input:invalid]:border-accent   group-has-[input:invalid]:bg-accent'
        >
          {isSubmitting ? 'Connecting...' : 'Connect'}
        </button>
      </form>
    </SeparatorBorder>
  )
}
