'use client'

import { Download, Images, LogOut, ShieldUser, Tag, Unplug } from 'lucide-react'
import MenuButton, { MenuButtonProps } from './IconButtonsMenu'
import { LogOutUser } from '@/components/api/authapi'
import { protectedAPI } from '@/components/api/jwt_apis/protected-api-client'
import { useProtected } from '@/components/api/jwt_apis/ProtectedProvider'
import { useRouter } from 'next/navigation'

interface Props {
  protApi: protectedAPI
}

export default function MenuButtons() {
  const protectedApi = useProtected()

  const logout = async () => {
    const result = await LogOutUser(protectedApi)
    if (result === 200) {
      router.push("/login")
    } else {
      console.error("Logout failed", result)
    }
  }

  const ButtonCss = "lg:grid grid-col justify-items-center bg-accent p-4"
  const router = useRouter()

  const Buttons = [
    { icon: Tag, label: "Tag Manager", loc: "" } satisfies MenuButtonProps,
    { icon: Download, label: "Upload From Disk", loc: "/upload_from_file" } satisfies MenuButtonProps,
    { icon: Unplug, label: "Connect Service", loc: "" } satisfies MenuButtonProps,
    { icon: ShieldUser, label: "Account settings", loc: "" } satisfies MenuButtonProps,
    { icon: Images, label: "Media Actions", loc: "" } satisfies MenuButtonProps,
    {
      icon: LogOut, label: "Log Out", loc: "", style: "bg-red-400/40 border-red-500",
      func: () => { logout() }
    } satisfies MenuButtonProps,
  ]


  return (
    <>
      {
        Buttons.map(({ icon, label, loc, style, func }, index) => (
          <MenuButton key={index} icon={icon} label={label} loc={loc} style={style} func={func} />
        ))
      }
    </>
  )
}