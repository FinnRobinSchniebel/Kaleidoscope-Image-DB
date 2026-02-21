import { FileArchive } from "lucide-react"
import { DragEvent, useEffect, useRef, useState } from "react"
import { SearchInfo } from "../../search/SearchBar"
import { UseFormRegister, UseFormSetValue, UseFormWatch } from "react-hook-form"
import { UploadFormValues } from "./page"

interface Props {
  MaxSize: number
  ValidFile: (e: boolean) => void
  FormRegister: UseFormRegister<UploadFormValues>
  SetFormValue: UseFormSetValue<UploadFormValues>
  watch: UseFormWatch<UploadFormValues>
}


export default function DropFile({ MaxSize, ValidFile, FormRegister, SetFormValue, watch}: Props) {

  const zipRegister = FormRegister("zipFile")

  const MAX_FILE_SIZE_Bytes = MaxSize * 1024 * 1024
  const inputRef = useRef<HTMLInputElement>(null)
  const [dragging, setDragging] = useState(false)
  const [fileName, setFileName] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const currentVal = watch("zipFile") != null

    ValidFile( currentVal ? currentVal : false)
  }, [error])

  const dropFileInBox = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    setDragging(false)

    const file = e.dataTransfer.files?.[0]
    if (!file || !inputRef.current) return

    const isZip = file.type === "application/zip" || file.name.toLowerCase().endsWith(".zip")
    if (!isZip) {
      setError("Only ZIP files are allowed")
      return
    }

    if (file.size > MAX_FILE_SIZE_Bytes) {
      setError(`File must be smaller than ${MaxSize}MB (Given ${file.size} MB)`)
      return
    }

    
    
    

    setFileName(file.name)
    SetFormValue("zipFile", file)

    setError(null)
    ValidFile(true)
  }

  return (
    <div
      className={`relative flex flex-col items-center justify-center w-full h-64 border-2 border-dashed rounded-xl cursor-pointer select-none transition-colors 
        ${dragging ? "border-blue-400 bg-blue-400/10" : "border-muted-foreground hover:border-primary-foreground hover:shadow-md shadow-primary/20"}`}
      onClick={() => inputRef.current?.click()}
      onDragOver={(e) => {
        e.preventDefault()
        setDragging(true)
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => dropFileInBox(e)}
    >

      <FileArchive className="w-12 h-12 text-accent-foreground mb-3" />


      <p className="text-primary-foreground font-medium">
        Drag & drop your ZIP file here
      </p>
      <p className="text-accent-foreground/70 text-sm mt-1">
        or click to browse
      </p>


      {fileName && (
        <p className="mt-3 text-sm  truncate max-w-[80%]">
          Current: <span className="italic">{fileName}</span>
        </p>
      )}
      {error && (
        <p className="text-red-600">{error}</p>
      )}


      <input
        type="file"
        accept=".zip,application/zip"
        className="hidden"
        {...FormRegister(`zipFile`)}
        ref={(el) => {
          zipRegister.ref(el)   // give ref to RHF
          inputRef.current = el // keep your DOM ref
        }}
        onChange={(e) => {
          const file = e.target.files?.[0]

          if (!file) return

          if (file.size > MAX_FILE_SIZE_Bytes) {
            setError(`File must be smaller than ${MaxSize} MB`)
            e.target.value = ""
            return
          }
          setFileName(file.name)
          setError(null)
          ValidFile(true)
          SetFormValue("zipFile", file)
        }}
      />
    </div>
  )
}