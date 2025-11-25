'use client'

import { useEffect, useState } from "react";
import SearchResults from "./Results";
import SearchBar from "./SearchBar";
import { protectedAPI } from "@/components/api/jwt_apis/protected-api-client";
import { ReadToken } from "@/components/api/get_variables_server";

interface Props{
  token : string
}

export default function Search(props: Props) {

  const [value, setValue] = useState<string[]>([]);

  let Protected : protectedAPI = CreateProtected(props.token)

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

function CreateProtected(token : string): protectedAPI {
  var p = new protectedAPI(token);
  console.log("test protected init ")
  return p
}