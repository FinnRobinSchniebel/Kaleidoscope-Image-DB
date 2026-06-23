'use client'

import { useProtected } from '@/components/api/jwt_apis/ProtectedProvider'
import MenuButtons from '../../../components/KscopeSharedUI/account/MenuButtons'
import { Download, Images, LogOut, ShieldUser, Tag, Unplug } from 'lucide-react'
import { MenuButtonProps } from '@/components/KscopeSharedUI/account/IconButtonsMenu'
import { LogOutUser } from '@/components/api/authapi'
import { useRouter } from 'next/navigation'


type Props = {}

export default function AccountLayout({ }: Props) {

  const protectedApi = useProtected()
  const router = useRouter()
  const logout = async () => {
    const result = await LogOutUser(protectedApi)
    if (result === 200) {
      router.push("/login")
    } else {
      console.error("Logout failed", result)
    }
  }
  const Buttons = [
    { icon: Tag, label: "Tag Manager", loc: "" } satisfies MenuButtonProps,
    { icon: Download, label: "Upload From Disk", loc: "/upload_from_file" } satisfies MenuButtonProps,
    { icon: Unplug, label: "Connect Service", loc: "/services" } satisfies MenuButtonProps,
    { icon: ShieldUser, label: "Account settings", loc: "" } satisfies MenuButtonProps,
    { icon: Images, label: "Media Actions", loc: "" } satisfies MenuButtonProps,
    {
      icon: LogOut, label: "Log Out", loc: "", style: "bg-red-400/40 border-red-500",
      func: () => { logout() }
    } satisfies MenuButtonProps,
  ]



  return (
    <>
      <div className='p-10 text-4xl  w-full'>Account</div>
      <div className='flex-1 w-full'>
        <div className='grid grid-cols-2 w-full py-20 gap-4 p-4'>
          <MenuButtons Buttons={Buttons}/>
        </div>

      </div>
    </>

  )
}