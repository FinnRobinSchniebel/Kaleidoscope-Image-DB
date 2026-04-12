import { Bookmark, Download, Ellipsis, SquarePlay } from "lucide-react";
import HitAreaButton from "../HitAreaButton";
import { useCallback, useContext, useEffect, useRef, useState } from "react";
import { HideUIContext, HitTestContext } from "../VerticalSetCarousel";

import MorePopup from "./MoreOptionsPopup";



interface Props {
  Disabled: boolean
  active: boolean
  id: string
}

export function SideButtons({ Disabled, active, id }: Props) {

  const HideUICtx = useContext(HideUIContext)
  const ButtonCtx = useContext(HitTestContext)

  const [isMoreOpen, setIsMoreOpen] = useState(false)

  var CanClick = !HideUICtx && active

  useEffect(() => {
    if (!active) {
      setIsMoreOpen(() => { ButtonCtx?.enable(true); return false })
    }
  }, [active])

  const ButtonRef = useRef<HTMLDivElement>(null)


  //TODO: switch to anchoring 
  return (
    <div className="absolute right-2 lg:right-10 bottom-1/30 lg:bottom-1/20 z-5  w-150">

      <HitAreaButton onHit={() => { }} active={CanClick} className={`justify-self-end rounded-full backdrop-blur-md bg-accent border-2 text-primary justify-items-center p-2 transition-opacity duration-300 ease-out ${HideUICtx ? "opacity-0" : "opacity-100"}`}>
        <Bookmark className="size-8" color="grey" />
      </HitAreaButton>
      <div className="py-2"></div>
      <HitAreaButton onHit={() => { }} active={CanClick} className={`justify-self-end rounded-full backdrop-blur-md bg-accent border-2 text-primary justify-items-center p-2 transition-opacity duration-300 ease-out ${HideUICtx ? "opacity-0" : "opacity-100"}`}>
        <SquarePlay className="size-8" color="grey" />
      </HitAreaButton>
      <div className="py-2"></div>
      <HitAreaButton onHit={() => { }} active={CanClick} className={`justify-self-end  rounded-full backdrop-blur-md bg-accent border-2 text-primary justify-items-center p-2 transition-opacity duration-300 ease-out ${HideUICtx ? "opacity-0" : "opacity-100"}`}>
        <Download className="size-8" color="grey" />
      </HitAreaButton>
      <div className="py-2"></div>

      <div className="w-15 [anchor-name:--more] justify-self-end">
        <HitAreaButton onHit={() => { setIsMoreOpen(e => { ButtonCtx?.enable(e); return !e }) }} id="MoreOptionsTarget" active={CanClick} zHight={1}
          className={` justify-self-end rounded-full backdrop-blur-md bg-accent border-2 text-primary justify-items-center p-2 transition-all duration-200 ease-out 
          ${HideUICtx ? "opacity-0" : "opacity-100"}
          ${isMoreOpen ? "rounded-l-none pl-4" : ""}
          `}
        >
          <Ellipsis className="size-8" />

        </HitAreaButton>
      </div>

      <MorePopup openState={isMoreOpen} id={id} setOpen={e => { setIsMoreOpen(e); ButtonCtx?.enable(!e) }} ></MorePopup>








    </div>

  )
}