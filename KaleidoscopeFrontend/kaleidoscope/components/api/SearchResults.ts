'use client'

import React from 'react';
import { ScrollArea } from "@/components/ui/scroll-area"
import ImageCard, { LoadingImageCard } from "../../app/(app)/search/ImageCards"
import { Fragment, Suspense, useEffect } from "react";
import { Separator } from "@radix-ui/react-separator";
import { searchAPI, SearchRequest, SetData } from "@/components/api/jwt_apis/search-api";
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { useInView } from 'react-intersection-observer';
import { Tags } from 'lucide-react';

type SearchPageCountProps = {
  protected: protectedAPI
  page: number
}



export async function SearchPageCountResults(props: SearchPageCountProps): Promise<{ imageSets: SetData[]; count: number }> {

  const request: SearchRequest = {
    pageCount: 8,
    skipCount: props.page * 8,
    protectedApiRef: props.protected
  }
  //Todo: add form data to request


  var result = await searchAPI(request)

  return {
    imageSets: result.imageSets ?? [],
    count: result.count ?? 0,
  }

}

type SearchSkipCOountProps = {
  api: protectedAPI
  skipNumber: number
  LoadNumber: number
}


export async function SearchSkipResults({ api, skipNumber, LoadNumber }: SearchSkipCOountProps): Promise<{ imageSets: SetData[]; count: number }> {
  const request: SearchRequest = {
    pageCount: LoadNumber,
    skipCount: skipNumber,
    protectedApiRef: api
  }

  var result = await searchAPI(request)

  return {
    imageSets: result.imageSets ?? [],
    count: result.count ?? 0,
  }

}