import { createBrowserRouter, RouterProvider, Navigate } from 'react-router-dom'
import BasicLayout from './layouts/Basic'
import Login from './pages/Login'
import Register from './pages/Register'
import NotFound from './pages/404'
import Storages from './pages/Storages'
import StorageCreateForm from './pages/Storages/StorageCreateForm'
import Files from './pages/Files'
import StorageWorkers from './pages/StorageWorkers'
import StorageWorkerCreateForm from './pages/StorageWorkers/StorageWorkerCreateForm'

const router = createBrowserRouter([
  { path: '/login', element: <Login /> },
  { path: '/register', element: <Register /> },
  {
    path: '/',
    element: <BasicLayout />,
    children: [
      { index: true, element: <Navigate to="/storages" replace /> },
      { path: 'storages', element: <Storages /> },
      { path: 'storages/register', element: <StorageCreateForm /> },
      { path: 'storages/:id/files/*', element: <Files /> },
      { path: 'storage_workers', element: <StorageWorkers /> },
      { path: 'storage_workers/register', element: <StorageWorkerCreateForm /> },
      { path: '*', element: <NotFound /> },
    ],
  },
])

export default function App() {
  return <RouterProvider router={router} />
}
