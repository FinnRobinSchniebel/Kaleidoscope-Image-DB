import { Lock } from "lucide-react";
import { AutoTagWithMatches } from "@/components/api/getAutoTagDetails-api";
import { Skeleton } from "@/components/ui/skeleton";

interface Props {
  systemTags: AutoTagWithMatches[] | null
}

export default function SystemTagsRow({ systemTags }: Props) {

  if (systemTags === null) {
    return (
      <div className="flex flex-wrap gap-2 m-5">
        <Skeleton className="h-7 w-28 rounded-md" />
        <Skeleton className="h-7 w-24 rounded-md" />
        <Skeleton className="h-7 w-32 rounded-md" />
      </div>
    )
  }

  if (systemTags.length === 0) return null

  return (
    <div className="flex flex-wrap gap-2 m-5">
      {systemTags.map(tag => (
        <div
          key={tag.Id}
          className="flex items-center gap-1.5 rounded-md border border-muted-foreground/30 bg-muted/40 px-3 py-1 text-sm font-medium"
        >
          <Lock className="size-3 text-muted-foreground" />
          {tag.Name} <span>| {tag.Count}</span>
        </div>
      ))}
    </div>
  )
}
