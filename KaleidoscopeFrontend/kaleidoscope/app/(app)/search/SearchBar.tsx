'use client'

import { Item } from '@/components/ui/item'
import React from 'react'

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
import { Label } from '@radix-ui/react-label'

import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Checkbox } from '@/components/ui/checkbox'
import { searchAPI, SearchRequest } from '@/components/api/jwt_apis/search-api'
import { protectedAPI } from '@/components/api/jwt_apis/protected-api-client'
import { useRouter } from 'next/navigation'


type Props = {
  protected: protectedAPI
}

export default function SearchBar(props : Props) {

  const route = useRouter()

  const SearchCaller = async () =>{
    const request : SearchRequest = {
      PageCount: 1000,
      PageNumber: 0,
      protectedApiRef: props.protected
    }

    var result = await searchAPI(request)
    console.log(result)
  }


  return (
    <div>
      <Item variant="outline" className='m-2 xl:m-10 justify-center bg-background/10'>
        <InputGroup className='xl:max-w-[50%] text-primary bg-background/30'>
          <InputGroupInput placeholder="Search..." className='text-xl'/>
          <InputGroupAddon>
            <SearchIcon className='text-primary'/>
          </InputGroupAddon>
          <InputGroupAddon align="inline-end">0 results</InputGroupAddon>
          <InputGroupAddon align="inline-end">
            <InputGroupButton variant="Gradient" className='text-primary' onClick={SearchCaller}>Search</InputGroupButton>
          </InputGroupAddon>          
        </InputGroup>
        <Popover>
          <PopoverTrigger asChild>
            <Button variant="Gradient" className='bg-transparent'>Settings</Button>
          </PopoverTrigger>
          <PopoverContent className="p-4 mt-4">
            <PopDownGroup/>
          </PopoverContent>
        </Popover>
      </Item>
    </div>
  )

}

interface PopDownItemProps {
  id: string;
  defaultState: boolean;
  label: string;
  Description: string;
}


function PopDownGroup(){

  const OptionItemsGroupOne = [
    { id: "titleCheck", defaultState: false, label: "Search Title", Description: "Add results for matching titles." },
    { id: "authorCheck", defaultState: false, label: "Search Author", Description: "Add results for matching author names." },
    { id: "tagsCheck", defaultState: true, label: "Search Tags", Description: "Add results for matching Tags." },
    { id: "sourceCheck", defaultState: false, label: "Search Source", Description: "Add results for source names that match search." },
  ]
  const OptionItemsGroupTwo = [
    { id: "PartialCheck", defaultState: true, label: "Partial Matches", Description: "Searching for incomplete and partial matches." },
    { id: "AndOr", defaultState: false, label: "Match One", Description: "Search for all that match one of Any part of the search" },
  ]
  


  return (
    <>
      {OptionItemsGroupOne.map(PopDownItem)}
    </>
  )
}

function PopDownItem({id, defaultState, label, Description}: PopDownItemProps) {
    return (
      <div key={"item-" + id} className='flex flex-col gap-6'>
        <Label id={"label-" + id} className="hover:bg-accent/50 flex items-start gap-3 rounded-lg border p-3 has-[[aria-checked=true]]:border-blue-600 has-[[aria-checked=true]]:bg-blue-50 dark:has-[[aria-checked=true]]:border-blue-900 dark:has-[[aria-checked=true]]:bg-blue-950">
          <Checkbox
            id={id}
            defaultChecked={defaultState}
            className="data-[state=checked]:border-blue-600 data-[state=checked]:bg-blue-600 dark:data-[state=checked]:border-blue-700 dark:data-[state=checked]:bg-blue-700"
          />
          <div  id={"Text-" + id} className="grid gap-1.5 font-normal">
            <p id={"labelText-" + id} className="text-sm leading-none font-medium">
              {label}
            </p>
            <p id={"Description-" + id} className="text-muted-foreground text-sm">
             {Description}
            </p>
          </div>
        </Label>
      </div>
    )
}