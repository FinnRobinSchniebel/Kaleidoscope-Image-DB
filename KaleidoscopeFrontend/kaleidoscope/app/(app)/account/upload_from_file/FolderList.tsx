import { CornerDownRight, FolderArchive } from "lucide-react"
import { UseFormRegister } from "react-hook-form";
import { UploadFormValues } from "./page";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";

// This file is for implementing the Zip, folders, and file layers for parsing the dropped in zip
// Its contents is sent to the backend and parsed for the special features in the pathname. 
// It also provides the checkbox for what layer to group by.

interface Props {
  Layers: number
  LoadedSchema?: string[]
  FormRegister: UseFormRegister<UploadFormValues>
  //DefaultGrouping: string
}

export default function FolderList({ Layers, LoadedSchema, FormRegister }: Props) {

  const cssRadio = 'h-4 w-4 accent-primary cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 ring-offset-background'

  return (
    <>

      {/* zip file level index=0*/}
      <div key={'zip'} className={`flex min-w-0 mx-1 items-center ml-[calc(var(--i)*1rem)] xl:ml-[calc(var(--i)*2rem)] `} style={{ "--i": 0 } as React.CSSProperties}>
        <label htmlFor={`folder-zip`} className='text-justify flex'>
          <FolderArchive className='p-2 size-10' />
          <span className="self-center-safe text-ellipsis overflow-hidden whitespace-nowrap">
            Zip File
          </span>
        </label>
        <input id={`folder-zip`} {...FormRegister(`structureZip`)} className='outline-3 field-sizing-content outline-accent rounded-sm px-5 mx-3 h-8 min-w-30 max-w-90 text-primary/80 focus:outline-primary-foreground/80 font-normal focus:text-primary'></input>
        <input type="radio" value={`0`} {...FormRegister("GroupingLevel")} className={cssRadio} />
      </div>

      {/* folder levels set by user index= 1 -> Layers*/}
      {Array.from({ length: Layers }, (_, index) =>
      (
        <div key={index} className={`flex min-w-0 mx-1 items-center ml-[calc(var(--i)*1rem)] xl:ml-[calc(var(--i)*2rem)]`} style={{ "--i": index + 1 } as React.CSSProperties}>
          <label htmlFor={`folder-${index}`} className='text-justify flex'>
            <CornerDownRight className='p-2 size-10' />
            <span className="self-center-safe text-ellipsis overflow-hidden whitespace-nowrap">
              Folder
            </span>
          </label>
          <input id={`folder-${index}`} {...FormRegister(`folders.${index}`)} className='outline-3 field-sizing-content outline-accent rounded-sm px-5 mx-3 h-8 min-w-30 max-w-90 focus:border-primary-foreground text-primary/80 focus:outline-primary-foreground/80 font-normal focus:text-primary'></input>
          <input type="radio" value={`${index + 1}`} {...FormRegister("GroupingLevel")} className={cssRadio} />
        </div>
      ))}

      {/* File Level, index = Layers + 1*/}
      <div key={'files'} className={`flex min-w-0 mx-1 items-center  ml-[calc(var(--i)*1rem)] xl:ml-[calc(var(--i)*2rem)]`} style={{ "--i": Layers + 2 } as React.CSSProperties}>
        <label htmlFor={`folder-Files`} className='text-justify flex'>
          <CornerDownRight className='p-2 size-10' />
          <span className="self-center-safe text-ellipsis overflow-hidden whitespace-nowrap">
            File(s)
          </span>
        </label>
        <input id={`folder-Files`} {...FormRegister(`files`) }
          className='outline-3 field-sizing-content outline-accent rounded-sm px-5 mx-3 h-8 min-w-30 max-w-90 font-normal text-primary/80 focus:outline-primary-foreground/80 focus:text-primary '></input>
        <input type="radio" value={`${Layers + 1}`} {...FormRegister("GroupingLevel")} className={cssRadio} />
      </div>


    </>
  )
}