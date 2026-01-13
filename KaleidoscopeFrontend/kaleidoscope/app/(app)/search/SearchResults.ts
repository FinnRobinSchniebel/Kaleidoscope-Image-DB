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
}



export default async function SearchResults(props: SearchProps): Promise<{ imageSets: SetData[]; count: number }> {

  const request: SearchRequest = {
    pageCount: 8,
    pageNumber: props.page,
    protectedApiRef: props.protected
  }
  //Todo: add form data to request


  var result = await searchAPI(request)

  return {
    imageSets: result.imageSets ?? [],
    count: result.count ?? 0,
  }

}