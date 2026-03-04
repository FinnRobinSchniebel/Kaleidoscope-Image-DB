import { Bookmark, Download, Ellipsis, SquarePlay } from "lucide-react";
import HitAreaButton from "./HitAreaButton";
import { useContext, useEffect } from "react";
import { HideUIContext } from "./VerticalSetCarousel";


interface Props {
  Disabled: boolean
  active: boolean
}

export function SideButtons({ Disabled, active }: Props) {

  const HideUICtx = useContext(HideUIContext)


  var CanClick = !HideUICtx && active


  //TODO: switch to anchoring 
  return (
    <div className="absolute right-2 lg:right-10 bottom-1/30 lg:bottom-1/20 z-5 space-y-5">


      <HitAreaButton onHit={() => { }} active={CanClick} className={`rounded-full bg-accent border-2 text-primary justify-items-center p-2 transition-opacity duration-300 ease-out ${HideUICtx ? "opacity-0" : "opacity-100"}`}>
        <Bookmark className="size-8" />
      </HitAreaButton>

      <HitAreaButton onHit={() => { }} active={CanClick} className={`rounded-full bg-accent border-2 text-primary justify-items-center p-2 transition-opacity duration-300 ease-out ${HideUICtx ? "opacity-0" : "opacity-100"}`}>
        <SquarePlay className="size-8" />
      </HitAreaButton>

      <HitAreaButton onHit={() => { }} active={CanClick} className={`rounded-full bg-accent border-2 text-primary justify-items-center p-2 transition-opacity duration-300 ease-out ${HideUICtx ? "opacity-0" : "opacity-100"}`}>
        <Download className="size-8" />
      </HitAreaButton>

      <HitAreaButton onHit={() => { }} active={CanClick} className={`rounded-full bg-accent border-2 text-primary justify-items-center p-2 transition-opacity duration-300 ease-out ${HideUICtx ? "opacity-0" : "opacity-100"}`}>
        <Ellipsis className="size-8" />
      </HitAreaButton>
    </div>

  )
}