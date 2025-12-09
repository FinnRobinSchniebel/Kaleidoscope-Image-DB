import { Checkbox } from "@/components/ui/checkbox";
import { FormField } from "@/components/ui/form";
import { Label } from "@/components/ui/label";


interface PopDownItemProps {
  id: string;
  label: string;
  Description: string;
}


export function PopDownGroup(form:{ form: any }) {

  const OptionItemsGroupOne = [
    { id: "titleCheck", label: "Search Title", Description: "Add results for matching titles." },
    { id: "authorCheck", label: "Search Author", Description: "Add results for matching author names." },
    { id: "tagsCheck", label: "Search Tags", Description: "Add results for matching Tags." },
    { id: "sourceCheck", label: "Search Source", Description: "Add results for source names that match search." },
  ]
  const OptionItemsGroupTwo = [
    { id: "PartialCheck", label: "Partial Matches", Description: "Searching for incomplete and partial matches." },
    { id: "AndOr", label: "Match One", Description: "Search for all that match one of Any part of the search" },
  ]



  return (
    <>
      {OptionItemsGroupOne.map((item) => <PopDownItem key={"PDI-" + item.id} item={item} form={form}/>)}
    </>
  )
}

function PopDownItem({item, form}: {item: PopDownItemProps, form: any} ) {
  return (
    <div key={"item-" + item.id} className='flex flex-col gap-6'>
      <FormField control={form.control} name={item.id} render={({ field }) => (
        <Label id={"label-" + item.id} className="hover:bg-accent/50 flex items-start gap-3 rounded-lg border p-3 has-[[aria-checked=true]]:border-blue-600 has-[[aria-checked=true]]:bg-blue-50 dark:has-[[aria-checked=true]]:border-blue-900 dark:has-[[aria-checked=true]]:bg-blue-950">
          <Checkbox
            id={item.id}
            checked={field.value}
            onCheckedChange={(checked) => {
              field.onChange(checked)
            }}
            className="data-[state=checked]:border-blue-600 data-[state=checked]:bg-blue-600 dark:data-[state=checked]:border-blue-700 dark:data-[state=checked]:bg-blue-700"
            {...field}
          />
          <div id={"Text-" + item.id} className="grid gap-1.5 font-normal">
            <p id={"labelText-" + item.id} className="text-sm leading-none font-medium">
              {item.label}
            </p>
            <p id={"Description-" + item.id} className="text-muted-foreground text-sm">
              {item.Description}
            </p>
          </div>
        </Label>
      )} />
    </div>
  )
}