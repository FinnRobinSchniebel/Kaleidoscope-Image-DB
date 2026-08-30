import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { X } from "lucide-react";
import { useAutoTagName } from "./AutoTagCache";


interface Props {
  tag: string
  color?: string
  count?: number
  variant?: 'muted' | 'default'
  className?: string
  onRemove?: () => void
  onClick?: () => void
}


export default function TagBadge({ tag, color, className, count, variant = 'default', onRemove, onClick }: Props) {

  const label = count != undefined ? `${tag} | ${count}` : tag

  return (
    <>
      <Badge
        variant="default"
        className={cn(
          "mr-2",
          "bg-foreground text-primary border-muted-foreground/20",
          className
        )}
        style={color ? { borderColor: color } : undefined}
      >
        {onClick ? 
        (
          <button
            type="button"
            className="bg-transparent border-0 p-0 m-0 font-inherit text-inherit cursor-pointer rounded-xs hover:underline underline-offset-2 transition-colors focus-visible:outline-none focus-visible:ring-ring/50 focus-visible:ring-[3px]"
            onClick={onClick}
          >
            {label}
          </button>
        ) 
        : 
        (
          label
        )}
        {onRemove && (
          <button
            type="button"
            aria-label={`Remove ${tag}`}
            onClick={(e) => { e.stopPropagation(); onRemove() }}
          >
            <X className="size-3" />
          </button>
        )}
      </Badge >
    </>
  )
}



interface ResolvedProps extends Omit<Props, 'tag'> {
  id: string
}

//Falls back to showing the raw id if it never resolves, as a debugging aid.
export function ResolvedTagBadge({ color, variant, className, onRemove, onClick, id }: ResolvedProps) {

  const lookup = useAutoTagName(id)

  if (lookup.status === "loading") {
    return <Skeleton className={cn("h-5 w-16 rounded-full mr-2", className)} />
  }

  if (lookup.status === "not-found") {
    return <TagBadge tag={id} color={color} variant={variant} className={className} onRemove={onRemove} onClick={onClick} />
  }

  return <TagBadge tag={lookup.name} color={color} variant={variant} className={className} onRemove={onRemove} onClick={onClick} />
}