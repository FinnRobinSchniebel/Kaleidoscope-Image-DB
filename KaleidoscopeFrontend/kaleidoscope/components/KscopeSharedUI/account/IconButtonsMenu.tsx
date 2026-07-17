'use client'

import { Button } from "@/components/ui/button";
import { Download, Icon, LucideProps } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { ForwardRefExoticComponent, RefAttributes } from "react";


export interface MenuButtonProps {
  icon?: ForwardRefExoticComponent<Omit<LucideProps, "ref"> & RefAttributes<SVGSVGElement>> | string
  label: string
  loc: string
  style?: string
  func?: (i : number) => void
  disabled? : boolean
}

export default function MenuButton({ icon: Icon, index, label, loc, style, disabled, func }: MenuButtonProps & { index: number }) {

  const ButtonCss = "lg:grid grid-col justify-items-center bg-accent p-4"
  const pathname = usePathname()

 

  if( Icon == null){
    return (
      <Button variant="outline" disabled={disabled} className={`${ButtonCss} ${style}`} onClick={() => func?.(index)}>

       
        <div>{label}</div>

      </Button>
    )
  }

  if (typeof Icon == "string") {
    return (
      <Button variant="outline" disabled={disabled} className={`${ButtonCss} ${style}`} onClick={() => func?.(index)}>

        <img className='xl:size-10 size-8' src={Icon} />
        <div>{label}</div>

      </Button>
    )
  }

  if (loc == "") {
    return (
      <Button variant="outline" disabled={disabled} className={`${ButtonCss} ${style}`} onClick={() => func?.(index)}>

        <Icon className='xl:size-10 size-8' />
        <div>{label}</div>

      </Button>
    )
  }

  return (
    <Button asChild variant="outline" disabled={disabled} className={`${ButtonCss} ${style}`}>
      <Link href={`${pathname}${loc}`}>
        <Icon className='xl:size-10 size-8' />
        <div>{label}</div>
      </Link>
    </Button>
  )
}