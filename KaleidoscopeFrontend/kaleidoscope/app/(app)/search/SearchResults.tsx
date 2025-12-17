'use client'

import React from 'react';
import { ScrollArea } from "@/components/ui/scroll-area"
import ImageCard, { LoadingImageCard } from "./ImageCards"
import { Fragment, Suspense, useEffect } from "react";
import { Separator } from "@radix-ui/react-separator";
import { searchAPI, SearchRequest, SetData } from "@/components/api/jwt_apis/search-api";
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { useInView } from 'react-intersection-observer';
import { Tags } from 'lucide-react';

type SearchProps = {
  protected: protectedAPI
  page: number
  OpenImageSet: (i: number)=>void
}



export default async function SearchResults(props: SearchProps) {

  const request: SearchRequest = {
    PageCount: 8,
    PageNumber: props.page,
    protectedApiRef: props.protected
  }

  console.log("test")

  var result = await searchAPI(request)

  console.log("request made")
  console.log(result)

  if (result.imageSets && result.imageSets.length > 0 ){
    return result.imageSets.map((item: SetData, index:number) =>(
      <ImageCard key={"card-" + item._id} id={item._id} Tags={item.tags} protAPI={props.protected} OpenImageSet={props.OpenImageSet}/>
    ))
  }

  return (
    []
  )

}
