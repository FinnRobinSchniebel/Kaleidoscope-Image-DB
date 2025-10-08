import { useState } from 'react'
import './index.css'
import Navlayout from './NavLayout.tsx'
import backGround from './assets/random Hexa.png'

function App() {
  //const [count, setCount] = useState(0)

  return (
    <div className=" w-full"> 
      <div className="relative h-dvh w-full overflow-hidden 4xl:w-6/10 justify-self-center">
        <img
          src={backGround}
          alt="background"
          className="absolute inset-0 w-full h-full object-cover"
        />
        <Navlayout></Navlayout>
      </div>
    </div>
  )
}

export default App
