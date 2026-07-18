'use client'

import React, { createContext, JSX, useRef, useState } from 'react';
import { useEffect } from "react";

import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { useInView } from 'react-intersection-observer';
import { searchPageCountResults } from '../../../components/api/searchResults';
import Image from 'next/image';
import { Dialog } from '@/components/ui/dialog';

import ImageSetDialog from '@/components/KscopeSharedUI/ImageSet/ImageSetDialog';
import { useProtected } from '@/components/api/jwt_apis/ProtectedProvider';
import { SetData } from '@/components/api/jwt_apis/search-api';
import ImageCard from './ImageCards';
import { useImageSetsProvider } from '@/components/KscopeSharedUI/ImageSet/ImageSetProvider';


type Props = {
  //imageSets: SetData[] | undefined;
}

type ImageSetAccess = {

}

//let empty = false

// export type ImageCard = JSX.Element

const imageSetAccess = createContext<ImageSetAccess | null>(null)

export default function LoadSearchResults(props: Props) {

  const { imageSets, loadNextPage } = useImageSetsProvider()

  const { ref, inView } = useInView()
  const RunningRef = useRef(false)


  const [open, setOpen] = useState(false)
  const [index, setIndex] = useState<number>(0)

  function openDialog(i: number) {
    setIndex(i)
    setOpen(true)
  }

  //Used when reaching the end of the page to load more
  useEffect(() => {

    let cancelled = false

    const run = async () => {

      while (!cancelled && inView) {

        const more = await loadNextPage()
        if (!more) break

        await new Promise(requestAnimationFrame)
      }
    }

    run()

    return () => {
      cancelled = true
    }


  }, [inView, loadNextPage])

  return (
    <>
      <section>
        <ul className="w-full grid grid-cols-2 md:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-1 pb-[15%] lg:pb-[6.5%] xl:pb-[4%] justify-center px-4">
          {imageSets.map((item: SetData, index: number) => (
            <ImageCard key={"card-" + item._id} id={item._id} index={index} Tags={item.tags} imageCount={item.activeImageCount} OpenImageSet={openDialog} />
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
        <ImageSetDialog imageSets={imageSets} index={index} />
      </Dialog>
    </>
  )
}


//grid-cols-2 md:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-5