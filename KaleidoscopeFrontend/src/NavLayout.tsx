import { useState } from 'react'
import { Home, Search, User, GalleryVertical, Grid2x2 } from "lucide-react";
import { Button } from "@/components/ui/button"
import './index.css'

import {
  NavigationMenu,
  NavigationMenuContent,
  NavigationMenuItem,
  NavigationMenuLink,
  NavigationMenuList,
  NavigationMenuTrigger,
  navigationMenuTriggerStyle,
} from "@/components/ui/navigation-menu"




function Layout() {
  //const [count, setCount] = useState(0)

  return (
    <>
      <div className='relative flex w-full h-full'>

      </div>


      <div className="absolute bottom-0 min-w-full h-20% bg-white/10 backdrop-blur-[2px] border-t border-white/20 p-2 z-50">
        <NavElements></NavElements>
      </div>
    </>
  )
}




function NavElements() {
  const navItems = [
    {icon: Home, label:"Home"},
    {icon: Grid2x2, label:"Grid"},
    {icon: GalleryVertical, label:"Feed"},
    {icon:User, label:"account"},
    {icon:Search, label:"Search"},
  ]
  const [activeIndex, setActiveIndex] = useState(0);

  return (
    <>
       <NavigationMenu className="w-full flex xl:max-w-6/10 justify-self-center">
          
            {navItems.map(({icon: Icon, label}, index) => (
              <NavigationMenuItem key={label} className='flex-1 flex flex-col items-center justify-center gap-1 cursor-pointer' onClick={() => setActiveIndex(index)}>
                <Icon className="justify-self-center" size={32} strokeWidth={2} />
                <span className="text-center">{label}</span>
              </NavigationMenuItem>
            ))}
          
        </NavigationMenu >      
    </>
  );
}


export default Layout

/* <div className="max-w-4xl mx-auto flex justify-around p-2">
          <Button  variant="default" className="py-6  flex-col">
            <Home size={64} strokeWidth={3.5}/>
            <span className="text-xs ">Home</span>
          </Button>
          <Button variant="outline" className="py-6  flex flex-col items-center text-gray-600 hover:text-blue-600">
            <Grid2x2 size={30} strokeWidth={1.6}/>
            <span className="text-xs">Grid View</span>
          </Button>

          <Button  variant="outline" className="py-6  flex flex-col items-center text-gray-600 hover:text-blue-600">
            <GalleryVertical size={24} />
            <span className="text-xs">Gallery</span>
          </Button>
        </div> */