import { cn } from "@/lib/utils"

interface Props {
  className?: string
}

export default function FadingSeparator({ className }: Props) {
  return (
    <div
      role="separator"
      className={cn("h-px mx-6 bg-linear-to-r from-transparent via-border/40 to-transparent", className)}
    />
  )
}
