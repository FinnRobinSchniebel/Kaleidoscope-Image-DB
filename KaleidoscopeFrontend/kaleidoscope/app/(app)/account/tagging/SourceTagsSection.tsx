import SourceGroupContainer, { SourceGroupSkeleton } from "./SourceGroupContainer";
import { SourceTagDoc } from "@/components/api/getSourceTags-api";

interface Props {
  sourceTagsBySource: Map<string, SourceTagDoc[]> | null
  onTagClick: (tag: SourceTagDoc) => void
}

export default function SourceTagsSection({ sourceTagsBySource, onTagClick }: Props) {

  if (!sourceTagsBySource) {
    return <>{Array.from({ length: 3 }).map((_, i) => <SourceGroupSkeleton key={i} />)}</>
  }

  if (sourceTagsBySource.size === 0) {
    return <p className="m-5 text-muted-foreground">No source tags imported yet.</p>
  }

  return (
    <>
      {Array.from(sourceTagsBySource.entries()).map(([source, tags]) => (
        <SourceGroupContainer key={source} source={source} tags={tags} onTagClick={onTagClick} />
      ))}
    </>
  )
}
