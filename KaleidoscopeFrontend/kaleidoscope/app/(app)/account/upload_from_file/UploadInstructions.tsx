import { Button } from "@/components/ui/button"
import { Collapsible, CollapsibleContent } from "@/components/ui/collapsible"
import { useState } from "react"

interface Props {
  MaxSize: number
}

export default function UploadInstructions({ MaxSize }: Props) {

  const sizeInGB = Math.round(MaxSize / 1024 * 10) / 10
  const MaxFileSize = sizeInGB > 1 ? `${sizeInGB} GB` : `${MaxSize} MB`
  const [instructionsOpen, setInstructionsOpen] = useState(false)


  return (
    <Collapsible  className={`bg-accent w-[90%] xl:w-[60%] place-self-center  rounded-2xl py-2 transition-transform ${instructionsOpen ? "" : "cursor-pointer"}`}
      open={instructionsOpen}
      onOpenChange={setInstructionsOpen}
      onClick={() => { if (!instructionsOpen) { setInstructionsOpen(true) } }}
    >
      <h2 className='text-2xl'>Instructions</h2>

      {!instructionsOpen && <p className='font-normal bg-accent w-fit justify-self-center m-2 p-2 rounded-2xl'>Click to show instructions...</p>}
      <CollapsibleContent className='font-normal py-2 flex flex-col p-10 transition-all duration-150 data-[state=open]:animate-collapsible-down data-[state=closed]:animate-collapsible-up overflow-hidden'>
        <div className='justify-start text-start   '>
          <ol type='1' className='list-decimal'>
            <li>
              Upload a zip file with all the images you want to add.
              <ol className='list-disc list-outside ml-5 mt-2'>
                <li className=''> The file contents should be organized into folders and files with meaningful names.</li>
                <li className=''> Kaleidoscope will group files by source ID or folder (depending on your choice).</li>
                <li className=''> The File cannot be larger than <span className="font-bold text-red-700">{MaxFileSize}</span>.</li>
              </ol>
            </li>
            <li>
              Using the options below the update describe the file formatting.
              Each level indicates the naming convention used by your folders.
              Adding levels will increase the depth of your folder structure with
              the lowest one always being the naming convention of the image files.
              <ol className='list-disc list-outside ml-5 mt-2'>
                <li className=''> Names for all Files:</li>
                <ol>
                  <li className="relative pl-4 before:absolute before:left-0 before:content-['-']">
                    <span className='text-blue-700'>'[Title]'</span> : Image Set Title
                  </li>
                  <li className="relative pl-4 before:absolute before:left-0 before:content-['-']">
                    <span className='text-blue-700'>'[Author]'</span> : Authors name
                  </li>
                  <li className="relative pl-4 before:absolute before:left-0 before:content-['-']">
                    <span className='text-blue-700'>'[AuthorId]'</span> : author's ID (if the source give the author an ID)
                  </li>
                  <li className="relative pl-4 before:absolute before:left-0 before:content-['-']">
                    <span className='text-blue-700'>'[ID]'</span> : Source ID
                  </li>
                  <li className="relative pl-4 before:absolute before:left-0 before:content-['-']">
                    <span className='text-blue-700'>'[Source]'</span> : Source name (ex: Pixiv, Twitter, Instagram)
                  </li>
                  <li className="relative pl-4 before:absolute before:left-0 before:content-['-']">
                    <span className='text-blue-700'>'[Date]'</span> : Date posted (02_02_2026)
                  </li>
                </ol>
                <li className=''> For Files only: </li>
                <ol>
                  <li className="relative pl-4 before:absolute before:left-0 before:content-['-']">
                    <span className='text-red-600'>'[Order]'</span> : Part of File Name used for determining the order. Will use alphabetical/numerical (a before b and 0 before 1)
                  </li>
                  <li className="relative pl-4 before:absolute before:left-0 before:content-['-']">
                    <span className='text-red-600'>'[-Order]'</span> : Part of File Name used for determining the order. Will use reverse alphabetical/numerical (b before a and 1 before 0)
                  </li>

                </ol>
              </ol>
            </li>
            <li>
              <p>
                Use the check Box at the end of the input to determine at what folder level images should be bundled together at.
              </p>
              <p className="italic ml-2">
                Ex: Checking files will have each file as there own Image set (one item per set), checking a folder level will bundle all individual folders at that level as separate image sets.
              </p>
            </li>
          </ol>


        </div>

        <Button onClick={() => setInstructionsOpen(false)} className='max-w-40 size-fit cursor-pointer mt-2'>close</Button>
      </CollapsibleContent>
    </Collapsible>
  )
}