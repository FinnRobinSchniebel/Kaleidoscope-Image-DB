import React, { useContext, useEffect, useRef } from "react"
import { HitTestContext } from "./VerticalSetCarousel"


interface Props {
  children?: React.ReactNode
  debugClassName?: string
  zHight?: number
  active: boolean
  id?: string
  onHit: () => void
  
}

export default function HitAreaButton({ children, debugClassName, active, zHight, id, onHit, ...props}: Props & React.HTMLAttributes<HTMLDivElement>) {


  const buttonRef = useRef<HTMLDivElement | null>(null)

  const hitCtx = useContext(HitTestContext)

  const idRef = useRef(Math.random().toString(36).slice(2))

  useEffect(() => {
    if (!hitCtx || !active) return

    const id = idRef.current
    const z = zHight? zHight : 0

    hitCtx.register({
      id: id,
      zHight: z,
      rect: () => buttonRef.current?.getBoundingClientRect() ?? null,
      onHit,
    })

    return () => hitCtx.unregister(id)
  }, [active, zHight, hitCtx, onHit])


  return (
    <div ref={buttonRef} id={id} style={props.style}  className={`pointer-events-none ${props.className} ${hitCtx?.debug ? debugClassName : ""}`}>
      {children}
    </div>
  )

}