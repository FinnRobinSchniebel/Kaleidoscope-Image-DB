import { FullImageSetData, SourceInfo } from "@/components/api/GetImageSetData-api";
import { Badge } from "@/components/ui/badge";
import { Collapsible, CollapsibleContent } from "@/components/ui/collapsible";


import { useContext, useLayoutEffect, useRef, useState } from "react";
import Source from "../SourceInfo";
import TagBadge from "./TagBadge";
import { HideUIContext } from "./VerticalSetCarousel";


interface props {
  info: FullImageSetData | undefined
}

export default function Description({ info }: props) {

  const [isDescriptionOpen, seIsDescriptionOpen] = useState(false)
  const [moreTagCount, setMoreTagCOunt] = useState(0)

  const tagContainerRef = useRef<HTMLDivElement>(null)

  const HideUICtx = useContext(HideUIContext)

  var tsDate: Date | undefined
  if (info?.DateAdded) {
    tsDate = new Date(info?.DateAdded)
  }
  else {
    tsDate = undefined
  }


  const countItemsInFirstLine = () => {
    if (!tagContainerRef.current) return 0

    const children = Array.from(tagContainerRef.current.children) as HTMLElement[]
    if (children.length === 0) return 0

    const firstLineTop = children[0].offsetTop
    return children.filter(el => Math.abs(el.offsetTop - firstLineTop) > 1).length
  }

  useLayoutEffect(() => {

    if (!tagContainerRef.current) return

    const observer = new ResizeObserver(() => {
      setMoreTagCOunt(countItemsInFirstLine())
    })

    observer.observe(tagContainerRef.current)

    return () => observer.disconnect()

  })



  return (
    <>
      <div key={`description-${info?.Id}`} className={`absolute left-1/2 -translate-x-1/2 w-full bottom-0 h-full pointer-events-none z-1 
        ${isDescriptionOpen && "bg-gradient-to-b from-20% from-primary-foreground/0 to-primary-foreground/70"}
        transition-all duration-300 ease-out
        ${HideUICtx
          ? "opacity-0 translate-y-2 pointer-events-none"
          : "opacity-100 translate-y-0 pointer-events-auto"}
        `}
      >
        <Collapsible
          className={`absolute left-1/2 -translate-x-1/2 w-full xl:max-w-[60%] bottom-0 pb-4 px-2 text-primary pointer-events-auto 
            ${!isDescriptionOpen && "bg-gradient-to-b from-20% from-primary-foreground/0 to-primary-foreground/40"}`}
          key={`Collapsible-${info?.Id}`}
          open={isDescriptionOpen}
          onOpenChange={seIsDescriptionOpen}
          onClick={() => seIsDescriptionOpen(!isDescriptionOpen)}
        >

          <div key={"NonCollapsibleDescription"} className="flex overflow-x-hidden overflow-y-hidden">
            <span ref={tagContainerRef} className={isDescriptionOpen ? "" : "w-fit h-6 overflow-x-hidden overflow-y-hidden"}>
              {info?.Tags.map((item: string, index: number) => (
                <TagBadge key={`tag-${item}`} tag={item} />
              ))}
            </span>
            {/* tag count beyond visible */}
            {!isDescriptionOpen && moreTagCount > 0 &&
              <span className="inline-block shrink-0 flex-auto ">
                <Badge className="mr-2">"+{moreTagCount}"</Badge>
              </span>}
          </div>
          <div className="flex overflow-x-hidden overflow-y-hidden">
            <div className={`overflow-hidden font-bold ${!isDescriptionOpen && "whitespace-nowrap"}`}>
              {/* title */}
              {info?.Title ?? "..."}
            </div>
            {!isDescriptionOpen &&
              <div className="ml-1 mr-1 inline-block shrink-0 px-1 bg-white/10 rounded-sm">
                more...
              </div>}
          </div>
          <CollapsibleContent className=" transition-all duration-300 ease-out">
              
            {/* Authors */}
            <div key={"CollapsibleDescription"}>
              <span className="">Authors: </span>
              {info?.Authors.map((author: string, index: number) => (
                <span key={`author-${author}`} className="font-bold underline">{author}
                  {index < info.Authors.length - 1 && ","}</span>
              )) ?? "Authors: N/A"}
            </div>
            {/* Description */}
            <div className="ml-5 whitespace-pre-wrap">
              {info?.Description ?? "..."}
            </div>

            {/* Source and source date */}
            <div className="text-primary/80 my-1">
              {info?.Sources.map((item: SourceInfo, index: number) => (
                <Source key={`source-${item.Id}`} source={item} />
              )) ?? "Source: N/A"}
            </div>

            {/* Download info */}
            <div className="text-primary text-sm font-thin italic my-1">
              Download Date: <span className=" font-normal"> {tsDate?.toDateString() ?? "N/A"}</span>
            </div>
          </CollapsibleContent>
        </Collapsible>
      </div>
    </>
  )

}