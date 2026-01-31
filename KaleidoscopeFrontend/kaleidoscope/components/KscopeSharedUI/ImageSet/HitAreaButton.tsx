import { useContext, useEffect, useRef } from "react"
import { HitTestContext } from "./VerticalSetCarousel"


interface Props {
  children?: React.ReactNode
  className?: string | null
  debugClassName?: string
  onHit: () => void

}

export default function HitAreaButton({ children, className, debugClassName, onHit }: Props) {


  const buttonRef = useRef<HTMLDivElement | null>(null)

  const hitCtx = useContext(HitTestContext)

  const idRef = useRef(Math.random().toString(36).slice(2))

  useEffect(() => {
    if (!hitCtx) return

    const id = idRef.current

    hitCtx.register({
      id,
      rect: () => buttonRef.current?.getBoundingClientRect() ?? null,
      onHit,
    })

    return () => hitCtx.unregister(id)
  }, [hitCtx, onHit])


  return (
    <div ref={buttonRef}  className={`pointer-events-none ${className} ${hitCtx?.debug ? debugClassName : ""}`}>
      {children}
    </div>
  )

}