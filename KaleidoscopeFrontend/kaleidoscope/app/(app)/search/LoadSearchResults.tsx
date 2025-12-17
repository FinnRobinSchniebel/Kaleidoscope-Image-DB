'use client'

import React, { JSX, useRef, useState } from 'react';
import { useEffect } from "react";

import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { useInView } from 'react-intersection-observer';
import SearchResults from './SearchResults';
import Image from 'next/image';
import { Dialog} from '@/components/ui/dialog';

import ImageSetDialog from '@/components/KscopeSharedUI/ImageSet/ImageSetDialog';
import { useProtected } from '@/components/api/jwt_apis/ProtectedProvider';

type Props = {
  //imageSets: SetData[] | undefined;
}



//let empty = false

export type ImageCard = JSX.Element

export default function LoadSearchResults(props: Props) {

  const protectedAPI = useProtected()

  const { ref, inView } = useInView()
  
  const [cards, setCards] = useState<ImageCard[]>([])
  const [isEmpty, setisEmpty] = useState<boolean>(false)
  const pageRef = useRef(0)
  
  //temp
  const [open, setOpen] = useState(false)
  const [index, setIndex] = useState<number | null>(null)

   function openDialog(i: number) {
    setIndex(i)
    setOpen(true)
  }


  useEffect(() => {
    const fn = async () => {
      console.log(pageRef.current)
      if (!isEmpty && inView) {
        const res = await SearchResults({ protected: protectedAPI, page: pageRef.current, OpenImageSet: openDialog})
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
        <ul className="w-full flex flex-wrap pb-[15%] lg:pb-[6.5%] xl:pb-[4%] justify-center">
          {cards}
          
        </ul>
      </section>
      <section className='justify-items-center'>
        <Image ref={ref} src="./file.svg"
        alt=""
        width={50}
        height={50}
        />
      </section>
      <Dialog  open={open} onOpenChange={setOpen}>
         <ImageSetDialog/>
      </Dialog>
    </>
  )
}


//grid-cols-2 md:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-5