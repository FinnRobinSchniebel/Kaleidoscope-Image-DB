import { useProtected } from "@/components/api/jwt_apis/ProtectedProvider"
import { SetData } from "@/components/api/jwt_apis/search-api"
import { SearchPageCountResults, SearchSkipResults } from "@/components/api/SearchResults"
import { createContext, useCallback, useContext, useRef, useState } from "react"

interface ImageSetsContextType {
  imageSets: SetData[]
  loadNextPage: () => Promise<boolean>
  removeSet: (id: string) => void
  reset: () => void
}

const ImageSetsContext = createContext<ImageSetsContextType | null>(null)

export function ImageSetsProvider({ children }: { children: React.ReactNode }) {

  const protectedAPI = useProtected()

  const [imageSets, setImageSets] = useState<SetData[]>([])
  const [isEmpty, setIsEmpty] = useState(false)

  const pageRef = useRef(0)
  const loadingRef = useRef(false)

  //Loads the next set of images when reaching the bottom of the list
  const loadNextPage = useCallback(async (): Promise<boolean> => {

    if (isEmpty || loadingRef.current) return false

    loadingRef.current = true

    const page = pageRef.current++   // reserve page immediately

    try {

      const res = await SearchPageCountResults({
        protected: protectedAPI,
        page
      })

      if (res.imageSets.length === 0) {
        setIsEmpty(true)
        return false
      }

      setImageSets(prev => [...prev, ...res.imageSets])

      return true

    } finally {
      loadingRef.current = false
    }

  }, [isEmpty, protectedAPI])

  const removeSet = useCallback(async (id: string): Promise<void> => {
    var setLength = imageSets.length
    var skip = setLength - 1
    const res = await SearchSkipResults({
      api: protectedAPI,
      skipNumber: skip,
      LoadNumber: 1
    })

    setImageSets(prev => {
      const removed = prev.filter(x => x._id !== id)
      if (res.imageSets.length == 0) return removed
      return [...removed, res.imageSets[0]]
    })
  }, [protectedAPI])

  function reset() {
    pageRef.current = 0
    setImageSets([])
    setIsEmpty(false)
  }

  return (
    <ImageSetsContext.Provider
      value={{
        imageSets,
        loadNextPage,
        removeSet,
        reset
      }}
    >
      {children}
    </ImageSetsContext.Provider>
  )
}

export function useImageSetsProvider() {
  const ctx = useContext(ImageSetsContext)
  if (!ctx) throw new Error("useImageSets must be used inside ImageSetsProvider")
  return ctx
}