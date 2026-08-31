import { createContext, useCallback, useContext, useEffect, useSyncExternalStore } from "react"

export type AutoTagLookup =
  | { status: "loading" }
  | { status: "resolved"; name: string; count: number }
  | { status: "not-found" }

const loadingSnapshot: AutoTagLookup = { status: "loading" }
const notFoundSnapshot: AutoTagLookup = { status: "not-found" }

//Per-ImageSetsProvider cache of resolved AutoTag id to name/count.
export class AutoTagCacheStore {
  private cache = new Map<string, { name: string; count: number } | null>() // null = confirmed not-found
  //Cached per id - useSyncExternalStore needs a stable reference until the value actually changes.
  private snapshots = new Map<string, AutoTagLookup>()
  private listeners = new Map<string, Set<() => void>>()
  private pending = new Set<string>()
  private flushScheduled = false

  constructor(private fetchIds: (ids: string[]) => Promise<{ Id: string; Name: string; Count: number }[] | undefined>) { }

  private notify(id: string) {
    this.snapshots.delete(id)
    this.listeners.get(id)?.forEach(cb => cb())
  }

  subscribe = (id: string, callback: () => void): (() => void) => {
    let set = this.listeners.get(id)
    if (!set) {
      set = new Set()
      this.listeners.set(id, set)
    }
    set.add(callback)
    return () => {
      set!.delete(callback)
      if (set!.size === 0) this.listeners.delete(id)
    }
  }

  getSnapshot = (id: string): AutoTagLookup => {
    const cached = this.snapshots.get(id)
    if (cached) return cached

    const entry = this.cache.get(id)
    const snapshot: AutoTagLookup =
      entry === undefined ? loadingSnapshot :
      entry === null ? notFoundSnapshot :
      { status: "resolved", name: entry.name, count: entry.count }

    this.snapshots.set(id, snapshot)
    return snapshot
  }

  //Shared by page pre-warm and on-demand misses so concurrent callers batch into one request.
  request(ids: string[]) {
    let added = false
    for (const id of ids) {
      if (!this.cache.has(id) && !this.pending.has(id)) {
        this.pending.add(id)
        added = true
      }
    }
    if (!added) return
    if (this.flushScheduled) return
    this.flushScheduled = true
    queueMicrotask(() => this.flush())
  }

  private async flush() {
    this.flushScheduled = false
    if (this.pending.size === 0) return
    const batch = Array.from(this.pending)
    this.pending.clear()

    const results = await this.fetchIds(batch)
    if (results === undefined) {
      //Left unresolved (not not-found) so a later request() retries.
      return
    }

    const foundIds = new Set<string>()
    for (const r of results) {
      this.cache.set(r.Id, { name: r.Name, count: r.Count })
      foundIds.add(r.Id)
      this.notify(r.Id)
    }
    for (const id of batch) {
      if (!foundIds.has(id)) {
        this.cache.set(id, null)
        this.notify(id)
      }
    }
  }
}

export const AutoTagCacheContext = createContext<AutoTagCacheStore | null>(null)

//Re-renders only when this id's resolved value changes, not on every cache update.
export function useAutoTagName(id: string): AutoTagLookup {
  const autoTagStore = useContext(AutoTagCacheContext)
  if (!autoTagStore) throw new Error("useAutoTagName must be used inside ImageSetsProvider")

  useEffect(() => {
    autoTagStore.request([id])
  }, [autoTagStore, id])

  const subscribe = useCallback(
    (onChange: () => void) => autoTagStore.subscribe(id, onChange),
    [autoTagStore, id]
  )
  const getSnapshot = useCallback(
    () => autoTagStore.getSnapshot(id),
    [autoTagStore, id]
  )

  return useSyncExternalStore(subscribe, getSnapshot)
}
