import { useProtected } from "@/components/api/jwt_apis/ProtectedProvider"
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client"
import { SearchFilter, SetData } from "@/components/api/search-api"
import { searchPageCountResults, SearchSkipResults } from "@/components/api/SearchResults"
import getAutoTagsByIds_api from "@/components/api/getAutoTagsByIds-api"
import { AutoTagCacheContext, AutoTagCacheStore } from "./AutoTagCache"
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react"

interface ImageSetsDataContextType {
  imageSets: SetData[]
  generation: number
}

interface ImageSetsActionsContextType {
  loadNextPage: () => Promise<boolean>
  removeSet: (id: string) => void
  reset: () => void
}

const ImageSetsDataContext = createContext<ImageSetsDataContextType | null>(null)
const ImageSetsActionsContext = createContext<ImageSetsActionsContextType | null>(null)

interface ImageSetsProviderProps {
  children: React.ReactNode
  filter?: SearchFilter
}

export function ImageSetsProvider({ children, filter }: ImageSetsProviderProps) {

  const protectedAPI = useProtected()
  const protectedApiRef = useRef<protectedAPI>(protectedAPI)
  protectedApiRef.current = protectedAPI

  const autoTagStoreRef = useRef<AutoTagCacheStore | undefined>(undefined)
  if (!autoTagStoreRef.current) {
    autoTagStoreRef.current = new AutoTagCacheStore(ids => getAutoTagsByIds_api({ ids, protectedApi: protectedApiRef.current }))
  }
  const autoTagStore = autoTagStoreRef.current

  const [imageSets, setImageSets] = useState<SetData[]>([])
  const imageSetsRef = useRef<SetData[]>(imageSets)
  imageSetsRef.current = imageSets

  const pageRef = useRef(0)
  const loadingRef = useRef(false)
  const isEmptyRef = useRef(false)
  const filterRef = useRef<SearchFilter | undefined>(filter)
  filterRef.current = filter
  //bumped by reset() so a fetch already in flight when a reset happens
  //can recognize its result is stale and discard it instead of appending
  //onto (or racing with) the freshly-reset list
  const searchIdRef = useRef(0)
  //reactive mirror of searchIdRef: mutating a ref alone doesn't retrigger
  //effects, so this lets other effects depend on "a reset just happened"
  const [generation, setGeneration] = useState(0)

  //Loads the next set of images when reaching the bottom of the list
  const loadNextPage = useCallback(async (): Promise<boolean> => {

    if (isEmptyRef.current) return false
    //another call is already in flight - not exhausted, just busy, so the
    //caller (e.g. the scroll loop) should retry rather than give up
    if (loadingRef.current) return true

    loadingRef.current = true

    const searchId = searchIdRef.current
    const page = pageRef.current++   // reserve page immediately

    try {

      const res = await searchPageCountResults({
        protected: protectedAPI,
        page,
        filter: filterRef.current
      })

      //a reset() happened while this fetch was in flight (e.g. the filter
      //changed) - this page no longer belongs to the current list, and the
      //reset's own loadNextPage() call already started a fresh page 0
      if (searchId !== searchIdRef.current) return true

      if (res.imageSets.length === 0) {
        isEmptyRef.current = true
        return false
      }

      setImageSets(prev => [...prev, ...res.imageSets])
      autoTagStore.request(res.imageSets.flatMap(s => s.tags))

      return true

    } finally {
      loadingRef.current = false
    }

  }, [protectedAPI, autoTagStore])

  const removeSet = useCallback(async (id: string): Promise<void> => {
    const skip = imageSetsRef.current.length - 1
    const res = await SearchSkipResults({
      api: protectedAPI,
      skipNumber: skip,
      LoadNumber: 1,
      filter: filterRef.current
    })

    setImageSets(prev => {
      const removed = prev.filter(x => x._id !== id)
      if (res.imageSets.length == 0) return removed
      return [...removed, res.imageSets[0]]
    })
    autoTagStore.request(res.imageSets.flatMap(s => s.tags))
  }, [protectedAPI, autoTagStore])

  const reset = useCallback(() => {
    searchIdRef.current++
    pageRef.current = 0
    isEmptyRef.current = false
    setImageSets([])
    setGeneration(searchIdRef.current)
  }, [])

  //Restart pagination from page 0 whenever the active search filter changes,
  //so a new query doesn't just append onto results loaded under the old one.
  useEffect(() => {
    reset()
    loadNextPage()
  }, [filter, loadNextPage, reset])

  const data = useMemo(() => ({ imageSets, generation }), [imageSets, generation])
  const actions = useMemo(
    () => ({ loadNextPage, removeSet, reset }),
    [loadNextPage, removeSet, reset]
  )

  return (
    <AutoTagCacheContext.Provider value={autoTagStore}>
      <ImageSetsActionsContext.Provider value={actions}>
        <ImageSetsDataContext.Provider value={data}>
          {children}
        </ImageSetsDataContext.Provider>
      </ImageSetsActionsContext.Provider>
    </AutoTagCacheContext.Provider>
  )
}

export function useImageSetsData() {
  const ctx = useContext(ImageSetsDataContext)
  if (!ctx) throw new Error("useImageSetsData must be used inside ImageSetsProvider")
  return ctx
}

export function useImageSetsActions() {
  const ctx = useContext(ImageSetsActionsContext)
  if (!ctx) throw new Error("useImageSetsActions must be used inside ImageSetsProvider")
  return ctx
}
