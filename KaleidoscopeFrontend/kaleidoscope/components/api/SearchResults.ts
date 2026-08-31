'use client'

import React from 'react';
import { ScrollArea } from "@/components/ui/scroll-area"
import ImageCard, { LoadingImageCard } from "../../app/(app)/search/ImageCards"
import { Fragment, Suspense, useEffect } from "react";
import { Separator } from "@radix-ui/react-separator";
import { searchAPI, SearchFilter, SearchRequest, SetData } from "@/components/api/search-api";
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { useInView } from 'react-intersection-observer';
import { Tags } from 'lucide-react';

function toRequestFilter(filter?: SearchFilter): SearchFilter {
  return {
    words: filter?.words ?? [],
    tags: filter?.tags ?? [],
    titles: filter?.titles ?? [],
    authors: filter?.authors ?? [],
    sources: filter?.sources ?? [],
    searchTags: filter?.searchTags ?? false,
    searchTitles: filter?.searchTitles ?? false,
    searchAuthors: filter?.searchAuthors ?? false,
    searchSources: filter?.searchSources ?? false,
    fromDate: filter?.fromDate,
    toDate: filter?.toDate,
  }
}

type SearchPageCountProps = {
  protected: protectedAPI
  page: number
  filter?: SearchFilter
}



export async function searchPageCountResults(props: SearchPageCountProps): Promise<{ imageSets: SetData[]; count: number }> {

  const request: SearchRequest = {
    ...toRequestFilter(props.filter),
    pageCount: 24,
    skipCount: props.page * 24,
    protectedApiRef: props.protected
  }

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
  filter?: SearchFilter
}


export async function SearchSkipResults({ api, skipNumber, LoadNumber, filter }: SearchSkipCOountProps): Promise<{ imageSets: SetData[]; count: number }> {
  const request: SearchRequest = {
    ...toRequestFilter(filter),
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