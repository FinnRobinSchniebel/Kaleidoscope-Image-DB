import { Bookmark, Download, Ellipsis, SquarePlay } from "lucide-react";
import HitAreaButton from "./HitAreaButton";
import { useContext, useEffect, useRef, useState } from "react";
import { HideUIContext } from "./VerticalSetCarousel";
import "../../../app/globals.css"
import "./sideButton.css"


interface Props {
  Disabled: boolean
  active: boolean
}

export function SideButtons({ Disabled, active }: Props) {

  const HideUICtx = useContext(HideUIContext)

  const [isMoreOpen, setIsMoreOpen] = useState(false)

  var CanClick = !HideUICtx && active

  const popoverRef = useRef<HTMLDivElement>(null)

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


      <HitAreaButton onHit={() => { popoverRef.current?.togglePopover() }} id="MoreOptionsTarget" active={CanClick} zHight={1} className={` justify-self-end anchor/more rounded-full backdrop-blur-md bg-accent border-2 text-primary justify-items-center p-2 transition-all duration-300 ease-out 
        ${HideUICtx ? "opacity-0" : "opacity-100"}
        ${isMoreOpen ? "rounded-l-none pl-4" : ""}
        `}>
        <Ellipsis className="size-8" />

      </HitAreaButton>
      <div className="anchored-bottom-left/more -mr-1 w-50 h-50">
        <div
          ref={ popoverRef}
          id="more-options"
          popover="auto"
          className={`
            absolute
            right-0
            bottom-0
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
          `}
        >
          <div className="flex flex-col gap-2">
            <button className="text-left hover:text-accent">Option 1</button>
            <button className="text-left hover:text-accent">Option 2</button>
            <button className="text-left hover:text-accent">Option 3</button>
          </div>

        </div>
      </div>









    </div>

  )
}