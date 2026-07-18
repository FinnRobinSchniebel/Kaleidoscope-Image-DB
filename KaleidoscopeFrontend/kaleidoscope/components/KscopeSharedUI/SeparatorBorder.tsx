import { cn } from "@/lib/utils"
import { ReactNode } from "react"

interface props {
    children: ReactNode
    className?: string
}





export default function SeparatorBorder({children, className}: props) {

    return (
        <div className={cn("border-2 shadow-sm border-accent rounded-md p-2 ", className)}>
            {children}
        </div>
    )
}