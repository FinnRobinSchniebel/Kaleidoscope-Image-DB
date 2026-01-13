import { Badge } from "@/components/ui/badge";


interface Props{
  tag: string
  color? : string
}


export default function TagBadge({tag, color}: Props) {


  return (
    <>
      <Badge className={`mr-2 ${color ?? ""}`}>{tag}</Badge>
    </>
  )
}