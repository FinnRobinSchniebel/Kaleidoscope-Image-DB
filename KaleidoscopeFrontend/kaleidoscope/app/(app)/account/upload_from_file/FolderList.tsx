import { CornerDownRight, FolderArchive } from "lucide-react"
import { UseFormRegister } from "react-hook-form";
import { UploadFormValues } from "./page";

interface Props {
  Layers: number
  LoadedSchema?: string[]
  FormRegister: UseFormRegister<UploadFormValues>
}

export default function FolderList({ Layers , LoadedSchema, FormRegister}: Props) {
  return (
    <>
      <div key={'zip'} className={`flex min-w-0 mx-1 items-center ml-[calc(var(--i)*1rem)] xl:ml-[calc(var(--i)*2rem)] `} style={{ "--i": 0 } as React.CSSProperties}>

        <label htmlFor={`folder-zip`} className='text-justify flex'>
          <FolderArchive className='p-2 size-10' />

          <span className="self-center-safe text-ellipsis overflow-hidden whitespace-nowrap">
            Zip File
          </span>
        </label>
        <input id={`folder-zip`} {...FormRegister(`structureZip`)} className='outline-3 field-sizing-content outline-accent rounded-sm px-5 mx-3 h-8 min-w-30 max-w-90 text-primary/80 focus:outline-primary-foreground/80 font-normal focus:text-primary'></input>
      </div>

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
        </div>
      ))}
      <div key={'files'} className={`flex min-w-0 mx-1 items-center  ml-[calc(var(--i)*1rem)] xl:ml-[calc(var(--i)*2rem)]`} style={{ "--i": Layers + 2 } as React.CSSProperties}>

        <label htmlFor={`folder-Files`} className='text-justify flex'>

          <CornerDownRight className='p-2 size-10' />
          <span className="self-center-safe text-ellipsis overflow-hidden whitespace-nowrap">
            File(s)
          </span>
        </label>
        <input id={`folder-Files`}
          {...FormRegister(`files`)}
          className='outline-3 field-sizing-content outline-accent rounded-sm px-5 mx-3 h-8 min-w-30 max-w-90 font-normal text-primary/80 focus:outline-primary-foreground/80 focus:text-primary '
          
        ></input>
      </div>


    </>
  )
}