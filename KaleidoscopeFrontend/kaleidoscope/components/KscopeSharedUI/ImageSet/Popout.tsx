import { stat } from "fs"
import { useEffect, useRef, useState } from "react"

interface Props {
  setOpen: (e: boolean) => void
  openState: boolean
}

export default function Popout({ setOpen, openState }: Props) {


  const popoverRef = useRef<HTMLDivElement>(null)
  const [WasOpen, setwasOpen] = useState(false)


  useEffect(() => {
    function handleClick(e: PointerEvent) {
      if(!WasOpen) return

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
                    backdrop-blur-md
                    bg-accent
                    p-3
                    shadow-lg
                    transition-all
                    duration-200
                    ${openState ? "scale-100" : "scale-0"}
                    }
                `}
      >
        <div className="flex flex-col gap-2">
          <button onClick={(e) => e.stopPropagation()} className="text-left hover:text-accent">Option 1</button>
          <button onClick={(e) => e.stopPropagation()} className="text-left hover:text-accent">Option 2</button>
          <button onClick={(e) => e.stopPropagation()} className="text-left hover:text-accent">Option 3</button>
        </div>


      </div>
    </>
  )
}