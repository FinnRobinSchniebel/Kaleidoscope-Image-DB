'use client'

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { createContext, ReactNode, useContext, useState } from "react";


interface Props {
  children: ReactNode
}


type DangerOptions = {
  title: string
  description?: string
  confirmText?: string
  cancelText?: string
}

type DangerContextType = (options: DangerOptions) => Promise<boolean>

const DangerAlertContext = createContext<DangerContextType | null>(null)

export default function AlertPopup({ children }: Props) {

  const [resolver, setResolver] = useState<(value: boolean) => void>()
  const [options, setOptions] = useState<DangerOptions | null>(null)
  const [isOpen, setOpen] = useState(false)

  const confirm: DangerContextType = (opts) => {
    setOptions(opts)
    setOpen(true)

    return new Promise<boolean>((resolve) => {
      setResolver(() => resolve)
    })
  }
  function handleClose(result: boolean) {
    setOpen(false)
    resolver?.(result)
  }

  return (
    <DangerAlertContext.Provider value={confirm}>
      {children}
      <AlertDialog open={isOpen} onOpenChange={setOpen}>
        <AlertDialogContent size="sm" className='overflow-hidden text-primary border-destructive bg-red-100/70 backdrop-blur-sm'>
          <AlertDialogHeader>
            <AlertDialogTitle> {options?.title ?? "Warning"}</AlertDialogTitle>
          </AlertDialogHeader>
          <AlertDialogDescription >
            {options?.description ?? "Dangerous Action"}
          </AlertDialogDescription>
          <AlertDialogFooter>
            <AlertDialogAction onClick={() => handleClose(true)} variant={"destructive"}>{options?.confirmText ?? "Delete"}</AlertDialogAction>
            <AlertDialogCancel onClick={() => handleClose(false)} className="border-blue-300/80 border-2">Cancel</AlertDialogCancel>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DangerAlertContext.Provider>
  )
}

export function useDangerAlert() {
  const ctx = useContext(DangerAlertContext)
  if (!ctx) throw new Error("useDangerAlert must be inside DangerAlertProvider")
  return ctx
}