import { ReactNode } from "react"

interface props {
    children: ReactNode
}





export default function SeparatorBorder({children}: props) {

    return (
        <div className="border-2 shadow-sm border-accent rounded-md p-2 ">
            {children}
        </div>
    )
}