import { useState } from "react";
import SeparatorBorder from "@/components/KscopeSharedUI/SeparatorBorder";
import TagBadge from "@/components/KscopeSharedUI/ImageSet/TagBadge";
import { ScrollArea, ScrollBar } from "@/components/ui/scroll-area";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Trash2 } from "lucide-react";
import { useDangerAlert } from "@/components/KscopeSharedUI/ImageSet/AlertPopup";
import { displaySourceTagText, SourceTagDoc } from "@/components/api/getSourceTags-api";
import AutoTagColorSlot from "./AutoTagColorSlot";
import AddSourceTagPopover from "./AddSourceTagPopover";

export type ApplyResult = { ok: true } | { ok: false, error: string }

interface Props {
  id: string | null
  initialName: string
  initialMatchKeys: string[]
  count: number
  sourceTags: SourceTagDoc[]
  sourceTagsByKey: Map<string, SourceTagDoc>
  otherNames: string[]
  onApply: (name: string, matchKeys: string[]) => Promise<ApplyResult>
  onDelete?: () => void
  onDiscardDraft?: () => void
}

function sameKeys(a: string[], b: string[]) {
  if (a.length !== b.length) return false
  const setB = new Set(b)
  return a.every(k => setB.has(k))
}

export default function AutoTagContainer({ id, initialName, initialMatchKeys, count, sourceTags, sourceTagsByKey, otherNames, onApply, onDelete, onDiscardDraft }: Props) {

  const confirm = useDangerAlert()

  const [savedName, setSavedName] = useState(initialName)
  const [savedMatchKeys, setSavedMatchKeys] = useState(initialMatchKeys)
  const [draftName, setDraftName] = useState(initialName)
  const [draftMatchKeys, setDraftMatchKeys] = useState(initialMatchKeys)
  const [isApplying, setIsApplying] = useState(false)
  const [applyError, setApplyError] = useState('')

  const isDirty = id === null || draftName !== savedName || !sameKeys(draftMatchKeys, savedMatchKeys)
  const isDuplicate = otherNames.some(n => n.trim().toLowerCase() === draftName.trim().toLowerCase())

  function handleAdd(key: string) {
    setDraftMatchKeys(prev => [...prev, key])
  }

  function handleRemove(key: string) {
    setDraftMatchKeys(prev => prev.filter(k => k !== key))
  }

  function handleCancel() {
    if (id === null) {
      onDiscardDraft?.()
      return
    }
    setDraftName(savedName)
    setDraftMatchKeys(savedMatchKeys)
    setApplyError('')
  }

  async function handleApply() {
    setApplyError('')
    setIsApplying(true)
    const result = await onApply(draftName.trim(), draftMatchKeys)
    setIsApplying(false)
    if (!result.ok) {
      setApplyError(result.error)
      return
    }
    setSavedName(draftName.trim())
    setSavedMatchKeys(draftMatchKeys)
  }

  async function handleDelete() {
    const ok = await confirm({
      title: `Delete ${savedName}`,
      description: "This removes the auto tag from every image set currently carrying it. This action cannot be undone.",
      confirmText: "Delete",
      cancelText: "Cancel"
    })
    if (!ok) return
    onDelete?.()
  }

  return (
    <SeparatorBorder className="flex flex-col gap-2 m-5 p-3 bg-accent">
      <div className="flex items-start justify-between gap-2">
        <div className="flex flex-1 items-center gap-2">
          <Input
            value={draftName}
            onChange={e => setDraftName(e.target.value)}
            placeholder="Auto tag name"
            className="max-w-xs font-bold"
          />
          <span className="text-sm text-muted-foreground shrink-0">{count} sets</span>
        </div>
        {id !== null && (
          <Button type="button" variant="ghost" size="icon-sm" aria-label="Delete auto tag" onClick={handleDelete} disabled={isApplying}>
            <Trash2 className="size-4" />
          </Button>
        )}
      </div>

      {isDuplicate && <p className="text-destructive text-sm">An auto tag with this name already exists.</p>}

      <AutoTagColorSlot />

      {/* <ScrollArea aria-orientation="horizontal" className="w-full overflow-x-auto pb-3"> */}
        <SeparatorBorder className="flex items-center size-fit flex-wrap">
          {draftMatchKeys.map(key => {
            const tag = sourceTagsByKey.get(key)
            if (!tag) return null
            return <TagBadge variant="muted" className="m-1" key={key} tag={displaySourceTagText(tag)} count={tag.Count} onRemove={() => handleRemove(key)} />
          })}
          <AddSourceTagPopover sourceTags={sourceTags} excludeKeys={draftMatchKeys} onAdd={handleAdd} />
        </SeparatorBorder>
        {/* <ScrollBar orientation="horizontal" className="h-2" />
      </ScrollArea> */}

      {applyError && <p className="text-destructive text-sm">{applyError}</p>}

      {isDirty && (
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" size="sm" onClick={handleCancel} disabled={isApplying}>Cancel</Button>
          <Button
            type="button"
            variant="default"
            size="sm"
            onClick={handleApply}
            disabled={isApplying || isDuplicate || draftName.trim() === ''}
          >
            {isApplying ? 'Applying...' : 'Apply'}
          </Button>
        </div>
      )}
    </SeparatorBorder>
  )
}
