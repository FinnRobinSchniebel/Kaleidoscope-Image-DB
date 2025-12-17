"use client"

import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  ReactNode,
} from "react"
import {
  useRouter,
  usePathname,
  useSearchParams,
} from "next/navigation"
import { protectedAPI } from "./protected-api-client"

type Props = {
  token: string
  children: ReactNode
}

const ProtectedContext = createContext<protectedAPI | null>(null)

export function ProtectedProvider({ token, children }: Props) {
  const router = useRouter()
  const pathname = usePathname()
  const params = useSearchParams()

  const redirectRef = useRef<() => void>(() => {})

  useEffect(() => {
    redirectRef.current = () => {
      router.push(`/login?from=${pathname}?${params.toString()}`)
    }
  }, [router, pathname, params])

  const protectedApi = useMemo(() => {
    return CreateProtected(token, () => redirectRef.current())
  }, [token])

  return (
    <ProtectedContext.Provider value={protectedApi}>
      {children}
    </ProtectedContext.Provider>
  )
}

export function useProtected() {
  const ctx = useContext(ProtectedContext)
  if (!ctx) {
    throw new Error("useProtected must be used within ProtectedProvider")
  }
  return ctx
}

function CreateProtected(token: string, onUnauthorized: () => void): protectedAPI {
  var p = new protectedAPI(token, onUnauthorized);
  console.log("protected re-init")
  return p
}