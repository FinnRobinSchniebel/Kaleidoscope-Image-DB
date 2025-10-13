import React from "react";
import Navlayout from './NavLayout.tsx'
import backGround from '../assets/random Hexa.png'
import { Outlet } from "react-router";


function RootLayout() {
  return (
    <div className="h-dvh bg-cover w-full" style={{ backgroundImage: `url(${backGround})` }}>
      
        <Outlet/>
        <Navlayout onSelectOption={()=>{}}/>
      
    </div>
  )
}

export default RootLayout


  // <div className="relative  w-full overflow-hidden 4xl:w-6/10 justify-self-center">
  //       /* <img
  //         src={backGround}
  //         alt="background"
  //         className="absolute inset-0 w-full h-full object-cover"
  //       /> */

