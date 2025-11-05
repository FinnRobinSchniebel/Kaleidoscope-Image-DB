import { NextRequest, NextResponse } from 'next/server';


export function middleware(request: NextRequest) {
  const isAuthenticated = request.cookies.get('refresh_token');
  

  const urlCopy = request.nextUrl.clone()


  if (!isAuthenticated && !(request.nextUrl.pathname === '/login' || request.url === '/register')) {
    
    urlCopy.pathname = '/login'
    
    urlCopy.searchParams.set('from', request.nextUrl.pathname); // Set the current path as 'from'
    return NextResponse.redirect(urlCopy);
  }
  return NextResponse.next();
}

export const config = {
  // matcher solution for public, api, assets and _next exclusion
  matcher: "/((?!api|static|.*\\..*|_next).*)",
};