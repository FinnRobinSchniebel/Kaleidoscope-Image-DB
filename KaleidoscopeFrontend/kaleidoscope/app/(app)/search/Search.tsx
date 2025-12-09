'use client'

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import SearchResults from "./SearchResults";
import SearchBar from "./SearchBar";
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { ReadToken } from "@/components/api/get_variables_server";
import { ReadonlyURLSearchParams, useRouter, useSearchParams } from "next/navigation";
import { AppRouterInstance } from "next/dist/shared/lib/app-router-context.shared-runtime";
import { usePathname } from "next/navigation";
import Cookies from 'js-cookie';
import { SearchRequest, SetData } from "@/components/api/jwt_apis/search-api";

interface Props {
  token: string
}

export default function Search(props: Props) {

  var params = useSearchParams()


  const [ImageSet, setImageSet] = useState<SetData[]>()
  useEffect(() => {
    try {
      setImageSet(sessionStorage.getItem("SearchTerm") == params.toString() ? JSON.parse(sessionStorage.getItem("SearchImageSets") ?? "") ?? undefined : undefined)
    }
    catch {

    }

  }, [params])


  const ImageSetItems = useCallback((results: SetData[] | undefined) => {
    //TODO: make it not update when no new data is available and add a pop up to let the user know
    setImageSet(results)
  }, [])

  const router = useRouter()
  var pathname = usePathname()


  const redirectRef = useRef<() => void>(() => {});
  useEffect(() => {
    redirectRef.current = () => {
      router.push(`/login?from=${pathname}?${params.toString()}`);
    };
  }, [router, pathname, params]);

  const Protected: protectedAPI = useMemo(() => {
    return CreateProtected(props.token, () => redirectRef.current())
  }, [props.token])

  return (
    <>
      <SearchBar protected={Protected} setImageSets={ImageSetItems} />
      <SearchResults imageSets={ImageSet} protected={Protected} />
    </>
  )

}

function CreateProtected(token: string, onUnauthorized: () => void): protectedAPI {
  var p = new protectedAPI(token, onUnauthorized);
  console.log("test protected init ")
  return p
}


// async function searchCaller(SearchValues: any, searchParams: ReadonlyURLSearchParams) {
//   //get form data for equest
//   const request: SearchRequest = {
//     PageCount: 1000,
//     PageNumber: 0,
//     protectedApiRef: props.protected
//   }

//   //fetch data
//   var result = await searchAPI(request)

//   //pass search results to parent
//   props.setImageSets(result.imageSets)



// }

// function SearchToURL(SearchValues: any, searchParams: ReadonlyURLSearchParams, router: AppRouterInstance) {
//   const newparams = new URLSearchParams(searchParams.toString())

//   Object.entries(SearchValues).forEach(([key, value]) => {
//     // Convert booleans to strings
//     if (typeof value === "boolean") {
//       newparams.set(key, value ? "true" : "false");
//     } else if (value != null) {
//       newparams.set(key, value.toString());
//     }
//   });

//   //set session storage to hold results
//   sessionStorage.setItem("SearchTerm", newparams.toString())
//   sessionStorage.setItem("SearchCount", result.count?.toString() ?? '0')
//   sessionStorage.setItem("SearchImageSets", JSON.stringify(result.imageSets))

//   //set url to search params
//   if (searchParams.toString() != newparams.toString()) {
//     router.push(`?${newparams.toString()}`, { scroll: false })
//   }
// }