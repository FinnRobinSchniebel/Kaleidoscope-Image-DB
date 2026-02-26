'use client'

import { Download, Images, ShieldUser, Tag, Unplug } from 'lucide-react'
import MenuButton, { MenuButtonProps } from './IconButtonsMenu'

export default function MenuButtons() {
  const ButtonCss = "lg:grid grid-col justify-items-center bg-accent p-4"


  const Buttons = [
    { icon: Tag, label: "Tag Manager", loc: ""} satisfies MenuButtonProps,
    { icon: Download, label: "Upload From Disk", loc: "/upload_from_file"} satisfies MenuButtonProps,
    { icon: Unplug, label: "Connect Service", loc: "" } satisfies MenuButtonProps,
    { icon: ShieldUser, label: "Account settings", loc: ""} satisfies MenuButtonProps,
    { icon: Images, label: "Media Actions", loc: ""} satisfies MenuButtonProps,
  ]


  return (
    <>
      {
        Buttons.map(({ icon, label, loc }, index) => (
          <MenuButton key={index} icon={icon} label={label} loc={loc} />
        ))
      }
    </>
  )
}