'use client'

import { Download, Images, ShieldUser, Unplug } from 'lucide-react'
import MenuButton, { MenuButtonProps } from './IconButtonsMenu'

export default function MenuButtons() {
  const ButtonCss = "lg:grid grid-col justify-items-center bg-accent p-4"


  const Buttons = [
    { Icon: Download, Label: "Upload From Disk" } satisfies MenuButtonProps,
    { Icon: Unplug, Label: "Connect Service" } satisfies MenuButtonProps,
    { Icon: ShieldUser, Label: "Account settings" } satisfies MenuButtonProps,
    { Icon: Images, Label: "Media Actions" } satisfies MenuButtonProps,
  ]


  return (
    <>
      {
        Buttons.map(({ Icon, Label }, index) => (
          <MenuButton key={index} Icon={Icon} Label={Label} />
        ))
      }
    </>
  )
}