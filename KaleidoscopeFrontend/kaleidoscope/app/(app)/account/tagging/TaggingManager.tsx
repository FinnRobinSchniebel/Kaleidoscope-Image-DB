'use client'

import { forwardRef, useEffect, useImperativeHandle, useMemo, useState } from "react";
import { useProtected } from "@/components/api/jwt_apis/ProtectedProvider";
import getSourceTags_api, { SourceTagDoc } from "@/components/api/getSourceTags-api";
import getAutoTagDetails_api, { AutoTagWithMatches } from "@/components/api/getAutoTagDetails-api";
import createAutoTag_api from "@/components/api/createAutoTag-api";
import updateAutoTag_api from "@/components/api/updateAutoTag-api";
import deleteAutoTag_api from "@/components/api/deleteAutoTag-api";
import regatherSourceTags_api, { RegatherSummary } from "@/components/api/regatherSourceTags-api";
import AlertPopup from "@/components/KscopeSharedUI/ImageSet/AlertPopup";
import FadingSeparator from "@/components/KscopeSharedUI/FadingSeparator";
import SourceTagsSection from "./SourceTagsSection";
import SystemTagsRow from "./SystemTagsRow";
import AutoTagList from "./AutoTagList";
import { ApplyResult } from "./AutoTagContainer";

export interface DraftAutoTag {
  tempId: string
  name: string
  srcTagKeyMatch: string[]
}

export interface TaggingManagerHandle {
  regather: () => Promise<RegatherSummary>
}

function resolveMatches(keys: string[], sourceTagsByKey: Map<string, SourceTagDoc>): SourceTagDoc[] {
  return keys
    .map(k => sourceTagsByKey.get(k))
    .filter((t): t is SourceTagDoc => t !== undefined)
}

export default forwardRef<TaggingManagerHandle>(function TaggingManager(_props, ref) {

  const protectedApi = useProtected()

  const [sourceTags, setSourceTags] = useState<SourceTagDoc[] | null>(null)
  const [autoTags, setAutoTags] = useState<AutoTagWithMatches[] | null>(null)
  const [drafts, setDrafts] = useState<DraftAutoTag[]>([])

  useEffect(() => {

    getSourceTags_api({ protectedApi }).then(tags => setSourceTags(tags ?? []))
  }, [])

  useEffect(() => {

    getAutoTagDetails_api({ protectedApi }).then(tags => {
      const sorted = (tags ?? []).slice().sort((a, b) => a.Name.localeCompare(b.Name))
      setAutoTags(sorted)
    })
  }, [])

  const sourceTagsByKey = useMemo(() => {
    const map = new Map<string, SourceTagDoc>()

    for (const t of sourceTags ?? []) map.set(t.Key, t)

    for (const tag of autoTags ?? []) {

      for (const match of tag.Matches) {
        if (!map.has(match.Key)) map.set(match.Key, match)
      }
    }
    return map
  }, [sourceTags, autoTags])

  const sourceTagsBySource = useMemo(() => {

    if (!sourceTags) return null
    const map = new Map<string, SourceTagDoc[]>()

    for (const t of sourceTags) {
      const group = map.get(t.Source)
      if (group) group.push(t)
      else map.set(t.Source, [t])
    }

    for (const group of map.values()) {
      group.sort((a, b) => b.Count - a.Count)
    }
    return map
  }, [sourceTags])

  const systemTags = useMemo(() => (autoTags ?? []).filter(t => t.System), [autoTags])
  const regularTags = useMemo(() => (autoTags ?? []).filter(t => !t.System), [autoTags])

  function handleCreateDraft() {
    setDrafts(prev => [{ tempId: crypto.randomUUID(), name: '', srcTagKeyMatch: [] }, ...prev])
  }

  function handleDiscardDraft(tempId: string) {
    setDrafts(prev => prev.filter(d => d.tempId !== tempId))
  }

  async function handleApplyDraft(tempId: string, name: string, matchKeys: string[]): Promise<ApplyResult> {

    const result = await createAutoTag_api({ name, srcTagKeyMatch: matchKeys, protectedApi })

    if (!result) return { ok: false, error: 'Failed to create auto tag. Please try again.' }
    if ('conflict' in result) return { ok: false, error: 'An auto tag with this name already exists.' }

    setDrafts(prev => prev.filter(d => d.tempId !== tempId))

    setAutoTags(prev => [
      { Id: result.id, Name: name, Matches: resolveMatches(matchKeys, sourceTagsByKey), Count: 0 },
      ...(prev ?? []),
    ])

    return { ok: true }
  }

  async function handleApplyExisting(id: string, name: string, matchKeys: string[]): Promise<ApplyResult> {
    const result = await updateAutoTag_api({ id, name, srcTagKeyMatch: matchKeys, protectedApi })

    if (typeof result === 'object' && 'conflict' in result) return { ok: false, error: 'An auto tag with this name already exists.' }
    if (!result) return { ok: false, error: 'Failed to update auto tag. Please try again.' }

    setAutoTags(prev => (prev ?? []).map(t => t.Id === id
      ? { ...t, Name: name, Matches: resolveMatches(matchKeys, sourceTagsByKey) }
      : t
    ))

    return { ok: true }
  }

  async function handleDelete(id: string) {

    const success = await deleteAutoTag_api({ id, protectedApi })
    if (!success) return

    setAutoTags(prev => (prev ?? []).filter(t => t.Id !== id))
  }

  async function handleRegather(): Promise<RegatherSummary> {

    const summary = await regatherSourceTags_api({ protectedApi })
    if (!summary) throw new Error('Failed to regather source tags.')

    const tags = await getSourceTags_api({ protectedApi })
    setSourceTags(tags ?? [])

    return summary
  }

  useImperativeHandle(ref, () => ({ regather: handleRegather }))

  return (
    <AlertPopup>
      <SourceTagsSection sourceTagsBySource={sourceTagsBySource} />
      <SystemTagsRow systemTags={systemTags} />
      <FadingSeparator className="my-2" />
      {autoTags === null ? (
        <p className="m-5 text-muted-foreground">Loading auto tags...</p>
      ) : (
        <AutoTagList
          autoTags={regularTags}
          drafts={drafts}
          sourceTags={sourceTags ?? []}
          sourceTagsByKey={sourceTagsByKey}
          onCreateDraft={handleCreateDraft}
          onApplyDraft={handleApplyDraft}
          onDiscardDraft={handleDiscardDraft}
          onApplyExisting={handleApplyExisting}
          onDelete={handleDelete}
        />
      )}
    </AlertPopup>
  )
})
