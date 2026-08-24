import { Lock } from "lucide-react";
import { AutoTagWithMatches } from "@/components/api/getAutoTagDetails-api";

interface Props {
  systemTags: AutoTagWithMatches[]
}

export default function SystemTagsRow({ systemTags }: Props) {

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
