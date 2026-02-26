import { useProtected } from '@/components/api/jwt_apis/ProtectedProvider'
import MenuButtons from '../../../components/KscopeSharedUI/account/MenuButtons'

type Props = {}

export default function AccountLayout({ }: Props) {


  

  return (
    <>
      <div className='p-10 text-4xl  w-full'>Account</div>
      <div className='flex-1 w-full'>
        <div className='grid grid-cols-2 w-full py-20 gap-4 p-4'>
          <MenuButtons />
        </div>

      </div>
    </>

  )
}