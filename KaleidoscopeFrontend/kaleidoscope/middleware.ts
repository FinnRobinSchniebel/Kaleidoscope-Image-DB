import { cookies } from 'next/headers';
import { NextRequest, NextResponse } from 'next/server';
import { use } from 'react';


export function middleware(request: NextRequest) {


  const isAuthenticated = request.cookies.has("session_token") //sessionStorage.getItem("session_token");
  

  const urlCopy = request.nextUrl.clone()

  if(isAuthenticated && request.nextUrl.pathname === '/login'){
    const redirectPeram = request.nextUrl.searchParams.get("from")
    urlCopy.pathname = redirectPeram ?? '/'
    
    return NextResponse.redirect( urlCopy)
  }

  // if ( request.cookies.has("from") && (request.nextUrl.pathname === '/login' || request.nextUrl.pathname  === '/register')) {
  //   console.log("adding return address " + request.cookies.get("from")?.value)
    
  //   urlCopy.searchParams.set('from', "search/"); // Set the current path as 'from'
  //   //return NextResponse.redirect(urlCopy);

  //   console.log("url search: " + urlCopy.toString())
    
  //   return NextResponse.rewrite(urlCopy)
  // }

  return NextResponse.next();
}

export const config = {
  // matcher solution for public, api, assets and _next exclusion
  matcher: "/((?!api|static|.*\\..*|_next).*)",
};