'use client'

import { Button } from "@/components/ui/button";
import { Download, Icon, LucideProps } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { ForwardRefExoticComponent, RefAttributes } from "react";


export interface MenuButtonProps {
  icon: ForwardRefExoticComponent<Omit<LucideProps, "ref"> & RefAttributes<SVGSVGElement>>
  label: string
  loc: string
  style?: string
  func?: () => void
}

export default function MenuButton({ icon: Icon, label, loc, style, func }: MenuButtonProps) {

  const ButtonCss = "lg:grid grid-col justify-items-center bg-accent p-4"
  const pathname = usePathname()


  if (loc == "") {
    return (
      <Button variant="outline" className={`${ButtonCss} ${style}`} onClick={func}>

        <Icon className='xl:size-10 size-8' />
        <div>{label}</div>

      </Button>
    )
  }

  return (
    <Button asChild variant="outline" className={`${ButtonCss} ${style}`}>
      <Link href={`${pathname}${loc}`}>
        <Icon className='xl:size-10 size-8' />
        <div>{label}</div>
      </Link>
    </Button>
  )
}