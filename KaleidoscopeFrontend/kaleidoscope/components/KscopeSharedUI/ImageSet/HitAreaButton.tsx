import { useContext, useEffect, useRef } from "react"
import { HitTestContext } from "./VerticalSetCarousel"


interface Props {
  children?: React.ReactNode
  className?: string | null
  debugClassName?: string
  zHight?: number
  active: boolean
  onHit: () => void

}

export default function HitAreaButton({ children, className, debugClassName, active, zHight, onHit }: Props) {


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
    <div ref={buttonRef}  className={`pointer-events-none ${className} ${hitCtx?.debug ? debugClassName : ""}`}>
      {children}
    </div>
  )

}