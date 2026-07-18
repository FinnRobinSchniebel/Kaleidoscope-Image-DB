import { Dispatch, SetStateAction, useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import ConnectPixiv from "./connectPixiv"
import ConnectService from "./connectService"
import SeparatorBorder from "@/components/KscopeSharedUI/SeparatorBorder"
import { useProtected } from "@/components/api/jwt_apis/ProtectedProvider"
import removeService_api from "@/components/api/removeService-api"
import syncService_api from "@/components/api/syncService-api"

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
  const [isRemoving, setIsRemoving] = useState(false)
  const [removeError, setRemoveError] = useState('')
  const [isSyncing, setIsSyncing] = useState(false)
  const [syncError, setSyncError] = useState('')

  async function handleRemove() {
    setRemoveError('')
    setIsRemoving(true)
    const success = await removeService_api(dialog.BackendName, protectedApi)
    setIsRemoving(false)
    if (success) {
      changeOpen(false)
    } else {
      setRemoveError('Failed to remove service. Please try again.')
    }
  }

  async function handleSync() {
    setSyncError('')
    setIsSyncing(true)
    const success = await syncService_api(dialog.BackendName, protectedApi)
    setIsSyncing(false)
    if (!success) {
      setSyncError('Failed to start sync. Please try again.')
    }
  }

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

          <p className="text-lg"><span className="font-bold ">Last Synced:</span> %tba%</p>
          <p className="text-lg"><span  className="font-bold">Next Sync in:</span> %tba% Hours</p>

          <button
            type="button"
            onClick={handleSync}
            disabled={isSyncing}
            className="mt-4 bg-accent border-1 border-amber-500/80 shadow-black shadow-xs p-2 rounded font-bold
             hover:shadow-sm
             active:bg-accent"
          >
            {isSyncing ? 'Starting Sync...' : 'Sync'}
          </button>
          {syncError && <p className="text-destructive text-sm mt-2">{syncError}</p>}
          <button
            type="button"
            onClick={handleRemove}
            disabled={isRemoving}
            className="mt-4 bg-accent border-1 border-destructive shadow-black shadow-xs p-2 rounded font-bold
             hover:shadow-sm
             active:bg-accent transition-colors"
          >
            {isRemoving ? 'Removing...' : 'Remove'}
          </button>
          {removeError && <p className="text-destructive text-sm mt-2">{removeError}</p>}

        </SeparatorBorder>


        <SeparatorBorder className="p-2">
          <h1 className="font-bold text-2xl text-center my-2">Add/Update Credentials</h1>
          {dialog.BackendName == "pixiv" && (
            <>
              <ConnectPixiv onOpenChange={changeOpen} currentOpenState={currentOpenState} />

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