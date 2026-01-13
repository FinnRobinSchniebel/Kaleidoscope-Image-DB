import { SourceInfo } from "../api/GetImageSetData-api";


interface props {
    source: SourceInfo
}


export default function Source({ source }: props) {

    var tsDate: Date | undefined
    if (source?.DateCreated) {
        tsDate = new Date(source?.DateCreated)
    }
    else {
        tsDate = undefined
    }

    return (
        <>
            {/* Source and source date */}
            Source: <span className="text-primary font-bold mr-1">{source.Name} </span> Post Date: <span className="text-primary font-bold"> {tsDate?.toDateString() ?? "N/A"}</span>

        </>
    )
}