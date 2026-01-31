import { Button } from "@/components/ui/button";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { SwiperClass } from "swiper/react";
import HitAreaButton from "./HitAreaButton";


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

  const debug = `bg-red-700/60`

  const cssArrows = `size-8 md:size-10 2xl:size-15 text-primary-foreground/50 opacity-70 drop-shadow-md w-full `

  const rightAreaColor = "bg-linear-to-l from-primary-foreground/0 from-60% to-primary-foreground/80"
  const leftAreaColor = "bg-linear-to-l from-primary-foreground/80 to-40% to-primary-foreground/0"

  return (
    <HitAreaButton onHit={() => { props.direction == "left" ? props.api?.slidePrev(0) : props.api?.slideNext(0) }}
      className={`absolute  top-0 h-full w-[20%] z-1 rounded-2 grid place-items-center 
        ${props.direction == "left" ?
          ` left-0 ${rightAreaColor}` :
          ` right-0 ${leftAreaColor}`} 
            `}
      debugClassName="bg-red-600/50"
    >
      <div className="w-full">
        {props.direction == "left" ?
          <img className={`${cssArrows} `} src={'/arrow-left.svg'} />
          :
          <img className={`${cssArrows} place-self-end `} src={'/arrow-right.svg'} />}
      </div>
    </HitAreaButton>
  )
}