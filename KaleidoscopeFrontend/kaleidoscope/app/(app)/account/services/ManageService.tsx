import { Dispatch, SetStateAction, useEffect, useRef, useState } from "react"
import { ServiceDialogOptions } from "./serviceDialog"
import { Field, FieldContent, FieldDescription, FieldLabel, FieldTitle } from "@/components/ui/field"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import SeparatorBorder from "@/components/KscopeSharedUI/SeparatorBorder"
import { useDangerAlert } from "@/components/KscopeSharedUI/ImageSet/AlertPopup"
import removeService_api from "@/components/api/removeService-api"
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client"
import syncService_api from "@/components/api/syncService-api"
import getSyncSchedule_api, { ServiceSyncInfo } from "@/components/api/getSyncSchedule-api"
import setSyncSchedule_api from "@/components/api/setSyncSchedule-api"


function isZeroDate(iso?: string) {
  return !iso || new Date(iso).getFullYear() <= 1
}

function formatLastSynced(lastSynced?: string) {
  if (isZeroDate(lastSynced)) return 'Never'
  return new Date(lastSynced as string).toLocaleString()
}

function formatNextSync(sync: ServiceSyncInfo | null) {
  if (!sync || !sync.sync_interval_hours) return 'Not scheduled'
  if (isZeroDate(sync.last_synced)) return 'Pending first sync'

  const nextRunMs = new Date(sync.last_synced as string).getTime() + sync.sync_interval_hours * 60 * 60 * 1000
  const hoursRemaining = (nextRunMs - Date.now()) / (60 * 60 * 1000)
  if (hoursRemaining <= 0) return 'Pending sync'
  return `${Math.ceil(hoursRemaining)} Hours`
}



interface Props {
  changeOpen: Dispatch<SetStateAction<boolean>>
  currentOpenState: boolean
  dialog: ServiceDialogOptions
  protectedApi: protectedAPI
}



export default function ManagerService({ currentOpenState, changeOpen, dialog, protectedApi }: Props) {

  const confirm = useDangerAlert()
  const [isRemoving, setIsRemoving] = useState(false)
  const [removeError, setRemoveError] = useState('')
  const [syncError, setSyncError] = useState('')
  const [syncIntervalHours, setSyncIntervalHours] = useState('0') //use as the local syncIntervalHours to compare against the server one in syncInfo
  const [syncInfo, setSyncInfo] = useState<ServiceSyncInfo | null>(null)
  const [isUpdating, setIsUpdating] = useState(false)
  const [updateError, setUpdateError] = useState('')
  const syncPollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const isSyncing = syncInfo?.syncing ?? false
  const savedSyncIntervalHours = String(syncInfo?.sync_interval_hours ?? 0)

  function stopSyncPoll() {
    if (syncPollRef.current) {
      clearInterval(syncPollRef.current)
      syncPollRef.current = null
    }
  }

  useEffect(() => {
    if (!currentOpenState) {
      stopSyncPoll()
      setSyncInfo(null)
      setSyncIntervalHours('0')
      return
    }
    getSyncSchedule_api(dialog.BackendName, protectedApi).then(info => {
      if (!info) return
      setSyncInfo(info)
      setSyncIntervalHours(String(info.sync_interval_hours ?? 0))
    })
    return () => stopSyncPoll()
  }, [currentOpenState])



  async function handleSync() {
    setSyncError('')
    setSyncInfo(prev => ({ ...(prev ?? {}), syncing: true }))
    const success = await syncService_api(dialog.BackendName, protectedApi)
    if (!success) {
      setSyncInfo(prev => ({ ...(prev ?? {}), syncing: false }))
      setSyncError('Failed to start sync. Please try again.')
      return
    }

    stopSyncPoll()
    syncPollRef.current = setInterval(async () => {
      const info = await getSyncSchedule_api(dialog.BackendName, protectedApi)
      if (!info) return
      setSyncInfo(info)
      if (!info.syncing) {
        stopSyncPoll()
      }
    }, 5000)
  }

  async function handleUpdate() {
    setUpdateError('')
    setIsUpdating(true)
    const success = await setSyncSchedule_api(dialog.BackendName, Number(syncIntervalHours), protectedApi)
    if (success) {
      const info = await getSyncSchedule_api(dialog.BackendName, protectedApi)
      if (info) {
        setSyncInfo(info)
        setSyncIntervalHours(String(info.sync_interval_hours ?? 0))
      }
    } else {
      setUpdateError('Failed to update sync schedule. Please try again.')
    }
    setIsUpdating(false)
  }

  async function handleRemove() {

    const ok = await confirm({
      title: `Remove ${dialog.ServiceName}`,
      description: "This will delete your stored credentials for this service and stop any current actions. This action cannot be undone.",
      confirmText: "Remove",
      cancelText: "Cancel"
    })
    if (!ok) return

    setRemoveError('')
    setIsRemoving(true)
    const { success, error } = await removeService_api(dialog.BackendName, protectedApi)
    setIsRemoving(false)
    if (success) {
      changeOpen(false)
    } else {
      setRemoveError(error || 'Failed to remove service. Please try again.')
    }
  }

  return (
    <SeparatorBorder className="p-2 mb-4 flex flex-col">
      <h1 className="font-bold text-2xl text-center my-2">Manage</h1>

      <p className="text-lg"><span className="font-bold ">Last Synced:</span> {formatLastSynced(syncInfo?.last_synced)}</p>
      <p className="text-lg"><span className="font-bold">Next Sync in:</span> {formatNextSync(syncInfo)}</p>



      <div>
        <label className='font-bold'>Sync Frequency</label>
        <RadioGroup
          value={syncIntervalHours}
          onValueChange={v => setSyncIntervalHours(v)}
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
        type="button"
        onClick={handleUpdate}
        disabled={isUpdating}
        className={`mt-4 bg-accent border-1 border-green-500/80 shadow-black shadow-xs p-2 rounded font-bold
             hover:shadow-sm
             active:bg-accent transition-opacity duration-200
             ${syncIntervalHours === savedSyncIntervalHours ? 'opacity-0 invisible pointer-events-none' : 'opacity-100'}`}
      >
        {isUpdating ? 'Updating...' : 'Update'}
      </button>
      {updateError && <p className="text-destructive text-sm mt-2">{updateError}</p>}


      <button
        type="button"
        onClick={handleSync}
        disabled={isSyncing}
        className="mt-4 bg-accent border-1 border-amber-500/80 shadow-black shadow-xs p-2 rounded font-bold
             hover:shadow-sm
             active:bg-accent
             disabled:shadow-none disabled:border-amber-900/40"
      >
        {isSyncing ? 'Sync in progress' : 'Sync'}
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
  )
}