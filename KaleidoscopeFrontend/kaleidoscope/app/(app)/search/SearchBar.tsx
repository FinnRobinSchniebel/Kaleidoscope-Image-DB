'use client'

import { Item } from '@/components/ui/item'
import React, { SetStateAction, use, useEffect } from 'react'

import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
  InputGroupText,
  InputGroupTextarea,
} from "@/components/ui/input-group"
import { SearchIcon } from 'lucide-react'

import { Button } from '@/components/ui/button'


import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

import { searchAPI, SearchRequest, SetData } from '@/components/api/jwt_apis/search-api'
import { protectedAPI } from '@/components/api/jwt_apis/protected-api-client'
import { useRouter, useSearchParams } from 'next/navigation'
import { useForm } from 'react-hook-form'
import { Form, FormField } from '@/components/ui/form'
import { PopDownGroup } from './SearchDropdown'




type Props = {
  protected: protectedAPI
  setImageSets: (results: SetData[] | undefined) => void;
}

export default function SearchBar(props: Props) {

  //const url = usePathname()
  const searchParams = useSearchParams()
  const router = useRouter()

  const form = useForm({
    defaultValues: {
      Search: (searchParams.get("SearchTerm") ?? ""),
      titleCheck: (searchParams.get("titleCheck")) === 'true',
      authorCheck: (searchParams.get("authorCheck")) === 'true',
      tagsCheck: (searchParams.get("tagsCheck") ?? "true") === 'true',
      sourceCheck: (searchParams.get("sourceCheck")) === 'true'
    }
  })

  //change this
  //we want to make this simply change the search params and return the querry and not call the fetch yet
  //the fetch will happen in the results section
  //one page will be 12 items for now
  //


  const SearchCaller = async () => {
    const SearchValues = form.getValues();


    //get form data for equest
    const request: SearchRequest = {
      PageCount: 12,
      PageNumber: 0,
      protectedApiRef: props.protected
    }

    //fetch data
    var result = await searchAPI(request)

    //pass search results to parent 
    props.setImageSets(result.imageSets)

    if (searchParams.toString() != sessionStorage.getItem("SearchTerm")) {
      // console.log(searchParams.toString())
      // console.log(sessionStorage.getItem("SearchTerm"))

      const newparams = new URLSearchParams(searchParams.toString())

      Object.entries(SearchValues).forEach(([key, value]) => {
        // Convert booleans to strings
        if (typeof value === "boolean") {
          newparams.set(key, value ? "true" : "false");
        } else if (value != null) {
          newparams.set(key, value.toString());
        }
      });

      //set session storage to hold results
      sessionStorage.setItem("SearchTerm", newparams.toString())
      sessionStorage.setItem("SearchCount", result.count?.toString() ?? '0')
      sessionStorage.setItem("SearchImageSets", JSON.stringify(result.imageSets))

      //set url to search params
      
      router.push(`?${newparams.toString()}`, { scroll: false })
      
    }
  }

  useEffect(() => {
    (async () => {
      if (searchParams.toString() != sessionStorage.getItem("SearchTerm")) {
        SearchCaller()
      }
    })()
  }, [form])

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(SearchCaller)} className="space-y-8">
        <Item variant="outline" className='m-2 xl:m-10 justify-center bg-background/10'>

          <FormField control={form.control} name="Search" render={({ field }) => (
            <InputGroup className='xl:max-w-[50%] text-primary bg-background/30'>
              <InputGroupInput placeholder="Search..." className='text-xl'  {...field} />

              <InputGroupAddon>
                <SearchIcon className='text-primary' />
              </InputGroupAddon>

              <InputGroupAddon align="inline-end" className='text-primary/90'>0 results</InputGroupAddon>

              <InputGroupAddon align="inline-end">
                <InputGroupButton variant="Gradient" className='text-primary' onClick={SearchCaller}>Search</InputGroupButton>
              </InputGroupAddon>

            </InputGroup>
          )} />

          <Popover>
            <PopoverTrigger asChild>
              <Button variant="Gradient" className='bg-transparent'>Settings</Button>
            </PopoverTrigger>

            <PopoverContent className="p-4 mt-4">
              <PopDownGroup form={form} />
            </PopoverContent>
          </Popover>
        </Item>
      </form>
    </Form>
  )

}
