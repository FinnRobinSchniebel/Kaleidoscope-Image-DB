import { Button } from "@/components/ui/button";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { SwiperClass } from "swiper/react";
import HitAreaButton from "./HitAreaButton";
import { useContext } from "react";
import { HideUIContext } from "./VerticalSetCarousel";


interface Props {
  api?: SwiperClass
  direction: "left" | "right"
  index: number
  Count: number
  //Debug?: boolean

}

export default function NavigationHorizontal(props: Omit<Props, "direction">) {
  return (
    <>
      {props.Count > 1 &&
        <>
          {props.index != 0 &&
            <NavigationButton api={props.api}  direction="left" ></NavigationButton>}
          {props.index != props.Count - 1 &&
            <NavigationButton api={props.api} direction="right" ></NavigationButton>}
        </>}
    </>
  )
}


export function NavigationButton(props: Omit<Props, "index" | "Count">) {

  const HideUICtx = useContext(HideUIContext)

  const debug = `bg-red-700/60`

  const cssArrows = `size-8 md:size-10 2xl:size-15 text-primary-foreground/50 opacity-70 drop-shadow-md w-full ${props.direction != "left" ? "place-self-end" : "" }`

  const rightAreaColor = props.direction == "left" ? "left-0 bg-linear-to-l from-primary-foreground/0 from-60% to-primary-foreground/80" : "right-0 bg-linear-to-l from-primary-foreground/80 to-40% to-primary-foreground/0"
  //const leftAreaColor =  HideUICtx ? " " : "bg-linear-to-l from-primary-foreground/80 to-40% to-primary-foreground/0"

  const arrowImage = props.direction == "left" ? '/arrow-left.svg': '/arrow-right.svg'


  return (
    <HitAreaButton onHit={() => { props.direction == "left" ? props.api?.slidePrev(0) : props.api?.slideNext(0) }}
      className={`absolute  top-0 h-full w-[20%] z-1 rounded-2 grid place-items-center transition-opacity duration-300 ease-out
        ${rightAreaColor} 
        ${HideUICtx ? "opacity-0" : "opacity-100"}
        
        `}
      debugClassName={debug}
    >
      <div className="w-full">
        <img className={`${cssArrows} transition-opacity duration-300 ease-out ${HideUICtx ?"opacity-0" : "opacity-100"} `} src={arrowImage} />
        
      </div>
    </HitAreaButton>
  )
}