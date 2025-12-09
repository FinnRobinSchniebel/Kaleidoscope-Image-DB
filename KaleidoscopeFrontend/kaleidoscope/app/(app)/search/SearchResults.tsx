'use client'

import { ScrollArea } from "@/components/ui/scroll-area"
import ImageCard, { LoadingImageCard } from "./ImageCards"
import { Fragment, Suspense, useEffect } from "react";
import { Separator } from "@radix-ui/react-separator";
import { SetData } from "@/components/api/jwt_apis/search-api";
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";


type Props = {
  protected: protectedAPI
  imageSets: SetData[] | undefined;
}

export default function SearchResults(props: Props) {

  let elementsArray = Array.from({ length: 30 }, (_, index) => (<li key={'li-' + index}> <Suspense fallback={<LoadingImageCard />}> <LoadingImageCard key={index} /> </Suspense> </li>));

  if (props.imageSets != undefined) {
    // elementsArray = props.imageSets?.map((set, index) => {
    //   return (
    //     <li key={'li-' + index}>
    //       <Suspense fallback={<LoadingImageCard />}>
    //         <ImageCard key={index} id={set.id} protAPI={props.protected} />
    //       </Suspense>

    //     </li>)
    // }, [props.protected])
  }


  useEffect(() => {
    (async () => {
      console.log("test results: ")
      console.log(props.imageSets)

    })()

  }, [props.imageSets])

  const tags = Array.from({ length: 50 }).map(
    (_, i, a) => `v1.2.0-beta.${a.length - i}`
  )

  return (

    <ul className="w-full flex flex-wrap pb-[15%] lg:pb-[6.5%] xl:pb-[4%] justify-center">

      {elementsArray}

    </ul>
  )
}

//grid-cols-2 md:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-5