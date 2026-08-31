'use client'

import { useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import Link from "next/link";
import { ChevronLeft, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import TaggingManager, { TaggingManagerHandle } from "./TaggingManager";
import { RegatherSummary } from "@/components/api/regatherSourceTags-api";




interface Props {

}


export default function page({ }: Props) {

  const managerRef = useRef<TaggingManagerHandle>(null)
  const [isRegathering, setIsRegathering] = useState(false)

  function handleRegather() {
    if (isRegathering || !managerRef.current) return

    setIsRegathering(true)
    const regathering = managerRef.current.regather().finally(() => setIsRegathering(false))

    toast.promise(regathering, {
      position: 'top-center',
      loading: 'Regathering source tags...',
      success: (summary: RegatherSummary) =>
        `Regathered: ${summary.created.length} created, ${summary.count_corrected.length + summary.translation_corrected.length} corrected`,
      error: 'Failed to regather source tags.',
    })
  }

  return (
    <>

      <h1 className='p-10 text-4xl'>Tag Manager</h1>

      <div className='flex flex-col flex-1 w-full'>
        <div className='flex items-center justify-between'>
          <Button className='m-4 w-fit bg-accent shadow-primary/60 hover:bg-accent/30' variant='outline' asChild>
            <Link href={`/account`}>
              <ChevronLeft></ChevronLeft>
              Back To Account
            </Link>
          </Button>
          <Button
            className='m-4 w-fit bg-accent shadow-primary/60 hover:bg-accent/30'
            variant='outline'
            disabled={isRegathering}
            onClick={handleRegather}
          >
            <RefreshCw className={isRegathering ? 'animate-spin' : ''} />
            Regather Source Tags
          </Button>
        </div>
        <TaggingManager ref={managerRef} />

      </div>

    </>
  )

}
