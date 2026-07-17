'use client'

import { Download, Images, LogOut, ShieldUser, Tag, Unplug } from 'lucide-react'
import MenuButton, { MenuButtonProps } from './IconButtonsMenu'
import { LogOutUser } from '@/components/api/authapi'
import { protectedAPI } from '@/components/api/jwt_apis/protected-api-client'
import { useProtected } from '@/components/api/jwt_apis/ProtectedProvider'
import { useRouter } from 'next/navigation'

interface Props {
  Buttons: MenuButtonProps[]
}

export default function MenuButtons({Buttons} : Props) {

  const ButtonCss = "lg:grid grid-col justify-items-center bg-accent p-4"
  

  


  return (
    <>
      {
        Buttons.map(({ icon, label, loc, style, func }, index) => (
          <MenuButton key={index} index={index} icon={icon} label={label} loc={loc} style={style} func={func} />
        ))
      }
    </>
  )
}