'use client'

import { Button } from "@/components/ui/button";
import { Download, Icon, LucideProps } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { ForwardRefExoticComponent, RefAttributes } from "react";


export interface MenuButtonProps {
  Icon: ForwardRefExoticComponent<Omit<LucideProps, "ref"> & RefAttributes<SVGSVGElement>>
  Label: string
}

export default function MenuButton({ Icon, Label }: MenuButtonProps) {

  const ButtonCss = "lg:grid grid-col justify-items-center bg-accent p-4"
  const pathname = usePathname()

  console.log(Label)

  return (
    <Button asChild variant="outline" className={`${ButtonCss}`}>
      <Link href={`${pathname}/upload_from_file`}>
        <Icon className='xl:size-10 size-8' />
        <div>{Label}</div>
      </Link>
    </Button>
  )
}