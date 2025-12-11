'use client'

import React, { JSX, useRef, useState } from 'react';
import { ScrollArea } from "@/components/ui/scroll-area"
import { LoadingImageCard } from "./ImageCards"
import { Fragment, Suspense, useEffect } from "react";
import { Separator } from "@radix-ui/react-separator";
import { searchAPI, SearchRequest, SetData } from "@/components/api/jwt_apis/search-api";
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { useInView } from 'react-intersection-observer';
import SearchResults from './SearchResults';
import Image from 'next/image';
import FirstPage from './firstPage';

type Props = {
  protected: protectedAPI
  //imageSets: SetData[] | undefined;
}



//let empty = false

export type ImageCard = JSX.Element

export default function LoadMore(props: Props) {


  const { ref, inView } = useInView()
  const [cards, setCards] = useState<ImageCard[]>([])
  const [isEmpty, setisEmpty] = useState<boolean>(false)
  const pageRef = useRef(1)
  

  useEffect(() => {
    const fn = async () => {
      console.log(pageRef.current)
      if (!isEmpty && inView) {
        const res = await SearchResults({ protected: props.protected, page: pageRef.current })
        console.log(res.length)
        if( res.length < 1) {
          setisEmpty(true)
        }
        setCards([...cards, ...res])
        pageRef.current++
      }
    }
    fn()
  }, [isEmpty, inView])

  return (
    <>
      <section>
        <ul ref={ref} className="w-full flex flex-wrap pb-[15%] lg:pb-[6.5%] xl:pb-[4%] justify-center">
          <FirstPage protected={props.protected}/>
          {cards}

        </ul>
      </section>
      <section>
        <Image src="./file.svg"
        alt=""
        width={50}
        height={50}
        />

      </section>
    </>
  )
}

//grid-cols-2 md:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-5