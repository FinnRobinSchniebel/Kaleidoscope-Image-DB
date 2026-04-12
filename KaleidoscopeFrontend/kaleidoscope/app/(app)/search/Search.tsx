'use client'

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import SearchBar, { SearchInfo } from "./SearchBar";
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { ReadToken } from "@/components/api/get_variables_server";
import { ReadonlyURLSearchParams, useRouter, useSearchParams } from "next/navigation";
import { AppRouterInstance } from "next/dist/shared/lib/app-router-context.shared-runtime";
import { usePathname } from "next/navigation";
import Cookies from 'js-cookie';
import { SearchRequest, SetData } from "@/components/api/jwt_apis/search-api";
import LoadSearchResults from "./LoadSearchResults";
import { ProtectedProvider } from "@/components/api/jwt_apis/ProtectedProvider";
import AlertPopup from "@/components/KscopeSharedUI/ImageSet/AlertPopup";
import { ImageSetsProvider } from "@/components/KscopeSharedUI/ImageSet/ImageSetProvider";


interface Props {
  token: string
}


export type ProtectedContext = protectedAPI


export default function Search(props: Props) {

  var params = useSearchParams()
  const router = useRouter()
  var pathname = usePathname()


  const [UserSearch, setUserSearch] = useState<SearchInfo>()
  const setSearch = useCallback((query: SearchInfo) => {
    if (JSON.stringify(query) != JSON.stringify(UserSearch)) {
      console.log("newSearch")


      setUserSearch(query)


      const newparams = new URLSearchParams(params.toString())

      Object.entries(query).forEach(([key, value]) => {
        // Convert booleans to strings
        if (typeof value === "boolean") {
          newparams.set(key, value ? "true" : "false");
        } else if (value != null) {
          newparams.set(key, value.toString());
        }
      });

      //router.push(`?${newparams.toString()}`, { scroll: false })
    }

  }, [UserSearch])



  //Create the Protected Route
  const redirectRef = useRef<() => void>(() => { });
  useEffect(() => {
    redirectRef.current = () => {
      router.push(`/login?from=${pathname}?${params.toString()}`);
    };
  }, [router, pathname, params]);




  return (
    <ProtectedProvider token={props.token}>
      <ImageSetsProvider>
        <SearchBar setSearchquery={setSearch} />
        <LoadSearchResults />
      </ImageSetsProvider>
    </ProtectedProvider>
  )

}
