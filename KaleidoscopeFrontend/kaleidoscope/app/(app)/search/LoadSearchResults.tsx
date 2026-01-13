'use client'

import React, { JSX, useRef, useState } from 'react';
import { useEffect } from "react";

import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { useInView } from 'react-intersection-observer';
import SearchResults from './SearchResults';
import Image from 'next/image';
import { Dialog } from '@/components/ui/dialog';

import ImageSetDialog from '@/components/KscopeSharedUI/ImageSet/ImageSetDialog';
import { useProtected } from '@/components/api/jwt_apis/ProtectedProvider';
import { SetData } from '@/components/api/jwt_apis/search-api';
import ImageCard from './ImageCards';

type Props = {
  //imageSets: SetData[] | undefined;
}



//let empty = false

// export type ImageCard = JSX.Element



export default function LoadSearchResults(props: Props) {


  const protectedAPI = useProtected()

  const { ref, inView } = useInView()

  const [ImageSets, setImageSets] = useState<SetData[]>([])
  const [count, setcount] = useState<number>(0)

  const [isEmpty, setisEmpty] = useState<boolean>(false)
  const pageRef = useRef(0)

  //temp
  const [open, setOpen] = useState(false)
  const [index, setIndex] = useState<number>(0)

  function openDialog(i: number) {
    setIndex(i)
    setOpen(true)
  }


  useEffect(() => {

    console.log("test running + " + pageRef.current + " " + inView)

    if (!inView || isEmpty) return

    const fn = async () => {
      console.log(pageRef.current)

      const res = await SearchResults({ protected: protectedAPI, page: pageRef.current })

      if (res.imageSets.length < 1) {

        console.log("is empty")
        setisEmpty(true)
        return
      }
      setImageSets([...ImageSets, ...res.imageSets])
      console.log(ImageSets)
      pageRef.current++
    }
    fn()
  }, [isEmpty, inView, ImageSets.length])

  return (
    <>
      <section>
        <ul className="w-full flex flex-wrap pb-[15%] lg:pb-[6.5%] xl:pb-[4%] justify-center">
          {ImageSets.map((item: SetData, index: number) => (
            <ImageCard key={"card-" + item._id} id={item._id} index={index} Tags={item.tags} OpenImageSet={openDialog} />
          ))}
        </ul>
      </section>
      <section className='justify-items-center'>
        <Image ref={ref} src="./file.svg"
          alt=""
          width={50}
          height={50}
        />
      </section>
      <Dialog open={open} onOpenChange={setOpen}>
        <ImageSetDialog imageSets={ImageSets} index={index} />
      </Dialog>
    </>
  )
}


//grid-cols-2 md:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-5