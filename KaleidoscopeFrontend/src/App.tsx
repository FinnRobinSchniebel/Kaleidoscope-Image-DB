
import './index.css'
import { Routes, Route, createBrowserRouter, createRoutesFromElements, RouterProvider } from 'react-router'
import Home from './PageLayouts/Home.tsx'
import SearchLayout from './PageLayouts/SearchLayout.tsx'
import AccountLayout from './PageLayouts/AccountLayout.tsx'
import BookmarksLayout from './PageLayouts/BookmarksLayout.tsx'
import ScrollFeedLayout from './PageLayouts/ScrollFeedLayout.tsx'
import RootLayout from './PageLayouts/RootLayout.tsx'


const routes = createBrowserRouter(
  createRoutesFromElements(
    <Route path='/' element={<RootLayout/>}>
      <Route index element={<Home/>}/>

      <Route path='search' element={<SearchLayout/>}/>
      <Route path='account' element={<AccountLayout/>}/>
      <Route path='bookmarks' element={<BookmarksLayout/>}/>
      <Route path='feed' element={<ScrollFeedLayout/>}/>
    </Route>      
  )
)


function App() {
  //const [count, setCount] = useState(0)

  return (    
      <RouterProvider router={routes}/>       
  )
}

export default App
