'use client'

import React, { JSX, useState } from 'react';
import { ScrollArea } from "@/components/ui/scroll-area"
import { LoadingImageCard } from "./ImageCards"
import { Fragment, Suspense, useEffect } from "react";
import { Separator } from "@radix-ui/react-separator";
import { searchAPI, SearchRequest, SetData } from "@/components/api/jwt_apis/search-api";
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { useInView } from 'react-intersection-observer';
import SearchResults from './SearchResults';
import Image from 'next/image';

type Props = {
  protected: protectedAPI
  //imageSets: SetData[] | undefined;
}


export type ImageCard = JSX.Element

export default function FirstPage(props: Props) {


  const [cards, setCards] = useState<ImageCard[]>([])

  useEffect(() => {
    const fn = async () => {
      console.log("ran")

      console.log("ran 2")
      const res = await SearchResults({ protected: props.protected, page: 0 })

      setCards(res)


    }
    fn()
  }, [])

  return (
    <>      
          {cards}
    </>
  )
}

//grid-cols-2 md:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-5