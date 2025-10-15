import { useState } from 'react'
import { Home, Search, User, GalleryVertical, Grid2x2, Bookmark, Tag } from "lucide-react";
import '../index.css'
import { cn } from "@/lib/utils"



import {
  NavigationMenu,
  NavigationMenuContent,
  NavigationMenuItem,
  NavigationMenuLink,
  NavigationMenuList,
  NavigationMenuTrigger,
  navigationMenuTriggerStyle,

} from "@/components/ui/navigation-menu"
import { NavLink } from 'react-router';


interface NavProps{
  onSelectOption: (item: string) => void;
}

function Layout(props: NavProps) {
  //const [count, setCount] = useState(0)

  const [activeIndex, setActiveIndex] = useState(0);

  const navItems = [
    { icon: Home, label: "Home", goto_link: "/" },
    //{ icon: Tag, label: "Tag" },
    { icon: GalleryVertical, label: "Feed", goto_link: "/feed" },
    { icon: Bookmark, label: "Saved", goto_link: "/bookmarks" },
    { icon: Search, label: "Search", goto_link: "/search" },
    { icon: User, label: "account", goto_link: "/account"},
  ]



  return (
    <>

      <div className="absolute bottom-0 min-w-full min-h-[5%] max-h-[12%]  backdrop-blur-[2px] border-t border-white/20 z-50">
        <NavigationMenu className="w-full h-full flex xl:max-w-6/10 justify-self-center">
          {navItems.map(({ icon: Icon, label, goto_link}, index) => (
            
            <NavigationMenuItem key={label}               
              className={'flex-1 flex flex-col items-center justify-center '}
            >

              <NavigationMenuLink asChild={true} className=''
                onClick={() => {
                  //setActiveIndex(index); 
                  props.onSelectOption(label);
                }}
              >

                <NavLink to={goto_link} className={({ isActive }) => cn('h-full w-full items-center justify-center cursor-pointer hover:bg-secondary/20 p-3', isActive ? "bg-accent/50" : "")}>
                  <Icon  key={label + "-icon"} className="size-auto text-foreground" strokeWidth={activeIndex === index ? 2.5 : 2}/>
                  <span key={label + "-label"} className={cn( activeIndex === index ? "font-bold" : "","text-center", "text-foreground")}>{label}</span>
                </NavLink>
               
              </NavigationMenuLink>

            </NavigationMenuItem>
          
          ))}

        </NavigationMenu >
      </div>
    </>
  )
}



//{({ isActive }) => cn('h-full w-full items-center justify-center cursor-pointer hover:bg-secondary/20 p-3', isActive ? "bg-accent/10" : "")}

// function NavElements(props: NavProps) {
  
  

//   return (
//     <>    
      
//     </>
//   );
// }


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