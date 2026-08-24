import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { X } from "lucide-react";


interface Props{
  tag: string
  color?: string
  count?: number
  variant?: 'default' | 'muted'
  className?: string
  onRemove?: () => void
}


export default function TagBadge({tag, color, className, count, variant = 'default', onRemove}: Props) {

  const label = count != undefined ? `${tag} | ${count}` : tag

  return (
    <>
      <Badge
        className={cn(
          "mr-2",
          variant === 'muted' && "bg-muted/50 text-muted-foreground border-muted-foreground/20",
          className
        )}
        style={color ? { borderColor: color } : undefined}
      >
        {label}
        {onRemove && (
          <button
            type="button"
            aria-label={`Remove ${tag}`}
            onClick={(e) => { e.stopPropagation(); onRemove() }}
          >
            <X className="size-3" />
          </button>
        )}
      </Badge>
    </>
  )
}