import { Dispatch, SetStateAction } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import ConnectPixiv from "./connectPixiv"
import ConnectService from "./connectService"
import SeparatorBorder from "@/components/KscopeSharedUI/SeparatorBorder"

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


  return (

    <Dialog open={currentOpenState} onOpenChange={changeOpen}>
      <DialogContent className='overflow-y-auto max-h-[90vh] bg-background/80 text-primary rounded-2 min-w-1/4 min-h-3/4 block' OverlayClassName='bg-black/40 backdrop-blur-[2px]'>
        <DialogHeader className='mb-1 mt-8 '>
          <DialogTitle className='font-bold text-3xl text-center'> {dialog.ServiceName} </DialogTitle>
        </DialogHeader>
        <DialogDescription className='text-muted-foreground size-fit my-2'>
          Connect and manage your {dialog.ServiceName} account
        </DialogDescription>

        <SeparatorBorder className="p-2 mb-4 flex flex-col">
          <h1 className="font-bold text-2xl text-center my-2">Manage</h1>
          <button
            type="button"
            className="mt-4 bg-amber-200/40 border-1 border-primary-foreground/50 shadow-black p-2 rounded font-bold
             hover:shadow-sm 
             active:bg-accent"
          >
            Sync
          </button>
          <button
            type="button"
            className="mt-4 bg-destructive border-1 border-destructive shadow-black shadow-xs p-2 rounded font-bold
             hover:shadow-sm
             active:bg-accent transition-colors"
          >
            Remove
          </button>
          
        </SeparatorBorder>


        <SeparatorBorder className="p-2">
          <h1 className="font-bold text-2xl text-center my-2">Add/Update Credentials</h1>
          {dialog.BackendName == "pixiv" && (
            <>
              <ConnectPixiv onOpenChange={changeOpen}></ConnectPixiv>

              <h1 className="font-bold text-center m-2  text-2xl">
                OR
              </h1>
            </>
          )
          }
          <ConnectService dialog={dialog} changeOpen={changeOpen} currentOpenState={currentOpenState} />
        </SeparatorBorder>

      </DialogContent>
    </Dialog >
  )
}