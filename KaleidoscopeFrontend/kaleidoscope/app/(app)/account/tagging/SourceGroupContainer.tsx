import SeparatorBorder from "@/components/KscopeSharedUI/SeparatorBorder";
import TagBadge from "@/components/KscopeSharedUI/ImageSet/TagBadge";
import { ScrollArea, ScrollBar } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { displaySourceTagText, SourceTagDoc } from "@/components/api/getSourceTags-api";

interface Props {
  source: string
  tags: SourceTagDoc[]
  onTagClick: (tag: SourceTagDoc) => void
}

export function SourceGroupSkeleton() {
  return (
    <SeparatorBorder className="flex items-center gap-3 m-5 p-3 bg-accent rounded-2xl">
      <Skeleton className="h-5 w-20 shrink-0" />
      <SeparatorBorder className="flex flex-1 min-w-0 border-none">
        <div className="flex flex-1 min-w-0 gap-2 overflow-hidden pb-3">
          <Skeleton className="h-5 w-20 rounded-full shrink-0" />
          <Skeleton className="h-5 w-28 rounded-full shrink-0" />
          <Skeleton className="h-5 w-16 rounded-full shrink-0" />
          <Skeleton className="h-5 w-24 rounded-full shrink-0" />
        </div>
      </SeparatorBorder>
    </SeparatorBorder>
  )
}

export default function SourceGroupContainer({ source, tags, onTagClick }: Props) {

  return (
    <SeparatorBorder className="flex items-center gap-3 m-5 p-3 bg-accent rounded-2xl">
      <span className="font-bold shrink-0 capitalize">{source}</span>
      <SeparatorBorder className="flex flex-1 min-w-0 border-none">
        <ScrollArea aria-orientation="horizontal" className="w-full overflow-x-auto pb-3">
          <div className="flex">
            {tags.map(t => (
              <TagBadge key={t.Key} tag={displaySourceTagText(t)} count={t.Count} variant="muted" onClick={() => onTagClick(t)} />
            ))}
          </div>
          <ScrollBar orientation="horizontal" className="h-2" />
        </ScrollArea>
      </SeparatorBorder>
    </SeparatorBorder>
  )
}
