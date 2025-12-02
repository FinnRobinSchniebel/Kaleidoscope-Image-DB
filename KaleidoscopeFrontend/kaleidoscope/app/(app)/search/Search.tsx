'use client'

import { useEffect, useState } from "react";
import SearchResults from "./Results";
import SearchBar from "./SearchBar";
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { ReadToken } from "@/components/api/get_variables_server";
import { useRouter, useSearchParams } from "next/navigation";
import { AppRouterInstance } from "next/dist/shared/lib/app-router-context.shared-runtime";
import { usePathname } from "next/navigation";
import Cookies from 'js-cookie';

interface Props{
  token : string
}

export default function Search(props: Props) {

  

  const [value, setValue] = useState<string[]>([]);

  const router = useRouter() 
  var pathname = usePathname()
  var params = useSearchParams()
  let Protected : protectedAPI = CreateProtected(props.token, () => { 
    router.push("/login?from=" + pathname + "?" + params)
  })

  //start the protected api
  // useEffect(() => {
  //   Protected = CreateProtected()

  // }, []);

  return (
    <>
      <SearchBar protected={Protected} />
      <SearchResults />
    </>
  )

}

function CreateProtected(token : string, onUnauthorized : ()=>void): protectedAPI {  
  var p = new protectedAPI(token, onUnauthorized);
  console.log("test protected init ")
  return p
}