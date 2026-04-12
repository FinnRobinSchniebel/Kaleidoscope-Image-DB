import { stat } from "fs"
import { useEffect, useRef, useState } from "react"
import { useDangerAlert } from "../AlertPopup"
import { useProtected } from "@/components/api/jwt_apis/ProtectedProvider"
import { deleteApi } from "@/components/api/delete-api"
import { toast } from "sonner"
import { useImageSetsProvider } from "../ImageSetProvider"

interface Props {
  setOpen: (e: boolean) => void
  openState: boolean
  id: string
}

export default function MorePopup({ setOpen, openState, id }: Props) {


  const { removeSet } = useImageSetsProvider()

  const popoverRef = useRef<HTMLDivElement>(null)
  const [WasOpen, setwasOpen] = useState(false)

  const Protected = useProtected()
  const confirm = useDangerAlert()

  async function handleDelete() {
    const ok = await confirm({
      title: "Delete Image",
      description: "This action cannot be undone.",
      confirmText: "Delete",
      cancelText: "Keep"
    })

    if (!ok) return

    const result = new Promise((resolve) =>
      setTimeout(() => resolve({ name: "Event" }), 2000)
    )

    //Deletes the imageset from the server. 
    var Deleted = deleteApi({ id, protectedApi: Protected })

    //Creates a toaster notification to indicate the state of the deletion
    toast.promise(Deleted, {
      position: "top-center",
      loading: "Loading...",
      success: `Item has been Deleted`,
      error: "Error",
      duration: 10000,
    })
    //removes image set from local image sets to update the change in the UI
    removeSet(id)
  }

  useEffect(() => {
    function handleClick(e: PointerEvent) {
      if (!WasOpen) return

      if (!popoverRef.current) return
      const target = e.target as Node

      if (!popoverRef.current.contains(target)) {
        setOpen(false)
      }


    }

    if (openState) {
      document.addEventListener("click", handleClick)
      setwasOpen(true)
    }

    return () => {
      document.removeEventListener("click", handleClick)
      setwasOpen(false)
    }
  }, [openState, WasOpen])


  return (
    <>
      <div
        id="more-options"
        ref={popoverRef}
        tabIndex={-1}
        className={`
          [position-anchor:--more] right-[anchor(left)] bottom-[anchor(bottom)] absolute m-0 -mr-1 
          z-55
          rounded-2xl
          border 
          rounded-br-none
          backdrop-blur-sm
          bg-background/40
          p-3
          shadow-lg
          transition-all
          duration-200
          ${openState ? "scale-100" : "scale-0"}
                    
                `}
      >
        <div className="gap-2 grid grid-cols-1 divide-solid divide-y-1 divide-primary-foreground">
          <button onClick={(e) => { e.stopPropagation(); handleDelete() }} className="  text-red-600 hover:text-red-400 pb-1">
            <p className="text-center font-bold">Delete</p>
          </button>
          <button onClick={(e) => e.stopPropagation()} className="  text-primary hover:text-accent pb-1">
            <p className=" text-center font-bold">Import Info</p>
          </button>

          <button onClick={(e) => e.stopPropagation()} className="decoration-solid text-center pb-1">WIP</button>
        </div>


      </div>
    </>
  )
}