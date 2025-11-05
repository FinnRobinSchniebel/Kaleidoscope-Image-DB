'use client'
import { useState } from 'react'
import { Home, Search, User, GalleryVertical, Grid2x2, Bookmark, Tag } from "lucide-react";
import './globals.css'
import { cn } from "@/lib/utils"
import Link from 'next/link';
import { useSelectedLayoutSegment } from "next/navigation";


import {
  NavigationMenu,
  NavigationMenuContent,
  NavigationMenuItem,
  NavigationMenuLink,
  NavigationMenuList,
  NavigationMenuTrigger,
  navigationMenuTriggerStyle,

} from "@/components/ui/navigation-menu"
import { log } from 'console';


function Layout() {
  //const [count, setCount] = useState(0)

  //const [pathname, setpathname] = useState(0);

  const navItems = [
    { icon: Home, label: "Home", goto_link: "/", ActiveString: "home" },
    //{ icon: Tag, label: "Tag" },
    { icon: GalleryVertical, label: "Feed", goto_link: "/feed", ActiveString: "feed" },
    { icon: Bookmark, label: "Saved", goto_link: "/bookmarks", ActiveString: "bookmarks" },
    { icon: Search, label: "Search", goto_link: "/search", ActiveString: "search" },
    { icon: User, label: "account", goto_link: "/account", ActiveString: "account" },
  ]

  const pathname = useSelectedLayoutSegment() ?? 'home';



  return (

    <>

      <div className="fixed bottom-0 min-w-full min-h-[5%] max-h-[12%]  backdrop-blur-[2px] border-t border-white/20 z-50">
        <NavigationMenu className="w-full h-full max-w-8/10 justify-self-center">
          {navItems.map(({ icon: Icon, label, goto_link, ActiveString }, index) => (

            <NavigationMenuItem key={label}
              className={'flex-1 flex flex-col items-center justify-center '}
            >

              <NavigationMenuLink asChild={true}

              >
                <Link href={goto_link} className={cn('h-full w-full items-center justify-center cursor-pointer hover:bg-secondary/20 p-3', pathname.startsWith(ActiveString) ? "bg-accent" : "")}>
                  <Icon key={label + "-icon"} className="size-auto text-primary" strokeWidth={pathname === ActiveString ? 3 : 2} />
                  <span key={label + "-label"} className={cn(pathname.startsWith(ActiveString) ? "font-bold" : "", "text-center", "text-primary")}>{label}</span>
                </Link>

              </NavigationMenuLink>

            </NavigationMenuItem>

          ))}

        </NavigationMenu >
      </div>
    </>
  )
}


export default Layout
