import { AutoTagWithMatches } from "@/components/api/getAutoTagDetails-api";
import { SourceTagDoc } from "@/components/api/getSourceTags-api";
import { Button } from "@/components/ui/button";
import { Plus } from "lucide-react";
import AutoTagContainer, { ApplyResult } from "./AutoTagContainer";
import { DraftAutoTag } from "./TaggingManager";

interface Props {
  autoTags: AutoTagWithMatches[]
  drafts: DraftAutoTag[]
  sourceTags: SourceTagDoc[]
  sourceTagsByKey: Map<string, SourceTagDoc>
  onCreateDraft: () => void
  onApplyDraft: (tempId: string, name: string, matchKeys: string[]) => Promise<ApplyResult>
  onDiscardDraft: (tempId: string) => void
  onApplyExisting: (id: string, name: string, matchKeys: string[]) => Promise<ApplyResult>
  onDelete: (id: string) => void
}

export default function AutoTagList({ autoTags, drafts, sourceTags, sourceTagsByKey, onCreateDraft, onApplyDraft, onDiscardDraft, onApplyExisting, onDelete }: Props) {

  const identities = [
    ...drafts.map(d => ({ key: d.tempId, name: d.name })),
    ...autoTags.map(a => ({ key: a.Id, name: a.Name })),
  ]

  function otherNamesFor(key: string) {
    return identities.filter(x => x.key !== key).map(x => x.name)
  }

  return (
    <div className="flex flex-col">
      <Button type="button" variant="outline" className="m-5 w-fit bg-accent shadow-primary/60 hover:bg-accent/30" onClick={onCreateDraft}>
        <Plus className="size-4" />
        New Auto Tag
      </Button>

      {drafts.map(draft => (
        <AutoTagContainer
          key={draft.tempId}
          id={null}
          initialName={draft.name}
          initialMatchKeys={draft.srcTagKeyMatch}
          count={0}
          sourceTags={sourceTags}
          sourceTagsByKey={sourceTagsByKey}
          otherNames={otherNamesFor(draft.tempId)}
          onApply={(name, matchKeys) => onApplyDraft(draft.tempId, name, matchKeys)}
          onDiscardDraft={() => onDiscardDraft(draft.tempId)}
        />
      ))}

      {autoTags.map(tag => (
        <AutoTagContainer
          key={tag.Id}
          id={tag.Id}
          initialName={tag.Name}
          initialMatchKeys={tag.Matches.map(m => m.Key)}
          count={tag.Count}
          sourceTags={sourceTags}
          sourceTagsByKey={sourceTagsByKey}
          otherNames={otherNamesFor(tag.Id)}
          onApply={(name, matchKeys) => onApplyExisting(tag.Id, name, matchKeys)}
          onDelete={() => onDelete(tag.Id)}
        />
      ))}
    </div>
  )
}
