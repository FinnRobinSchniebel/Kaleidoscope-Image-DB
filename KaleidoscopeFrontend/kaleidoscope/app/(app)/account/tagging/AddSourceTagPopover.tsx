'use client'

import { useMemo, useState } from "react"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command"
import { badgeVariants } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
import { Plus } from "lucide-react"
import { displaySourceTagText, SourceTagDoc } from "@/components/api/getSourceTags-api"

const MAX_RESULTS = 50

interface Props {
  sourceTags: SourceTagDoc[]
  excludeKeys: string[]
  onAdd: (key: string) => void
}

export default function AddSourceTagPopover({ sourceTags, excludeKeys, onAdd }: Props) {

  const [popoverOpen, setPopoverOpen] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [search, setSearch] = useState('')

  const results = useMemo(() => {
    const excluded = new Set(excludeKeys)
    const query = search.trim().toLowerCase()
    return sourceTags
      .filter(t => !excluded.has(t.Key) && (query === '' || displaySourceTagText(t).toLowerCase().includes(query)))
      .sort((a, b) => b.Count - a.Count)
      .slice(0, MAX_RESULTS)
  }, [sourceTags, excludeKeys, search])

  function handleSelect(key: string) {
    onAdd(key)
    setPopoverOpen(false)
    setDialogOpen(false)
    setSearch('')
  }

  // bg-transparent: PopoverContent/DialogContent already supply the panel's
  // background - Command's own default bg-popover would otherwise stack a
  // second translucent layer on top, making the panel more opaque and its
  // text lower-contrast than intended.
  const searchList = (
    <Command shouldFilter={false} className="bg-transparent">
      <CommandInput value={search} onValueChange={setSearch} placeholder="Search source tags..." />
      <CommandList>
        <CommandEmpty>No matching source tags.</CommandEmpty>
        <CommandGroup>
          {results.map(t => (
            <CommandItem key={t.Key} value={t.Key} onSelect={() => handleSelect(t.Key)}>
              {displaySourceTagText(t)}
              <span className="ml-auto text-xs text-muted-foreground">{t.Source} | {t.Count}</span>
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </Command>
  )

  const triggerClassName = cn(badgeVariants({ variant: "outline" }), "mr-2 cursor-pointer")

  return (
    <>
      <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
        <PopoverTrigger className={cn(triggerClassName, "hidden sm:inline-flex")} aria-label="Add source tag">
          <Plus className="size-3" />
        </PopoverTrigger>
        <PopoverContent className="p-0 w-80" align="start">
          {searchList}
        </PopoverContent>
      </Popover>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogTrigger className={cn(triggerClassName, "sm:hidden")} aria-label="Add source tag">
          <Plus className="size-3" />
        </DialogTrigger>
        <DialogContent
          className="h-dvh w-dvw max-w-full rounded-none gap-0 p-0 pt-14 bg-popover backdrop-blur-sm"
          showCloseButton
        >
          <DialogHeader className="sr-only">
            <DialogTitle>Add source tag</DialogTitle>
          </DialogHeader>
          {searchList}
        </DialogContent>
      </Dialog>
    </>
  )
}
