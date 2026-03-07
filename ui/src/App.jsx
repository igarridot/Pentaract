import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import BasicLayout from './layouts/Basic'
import Login from './pages/Login'
import Register from './pages/Register'
import NotFound from './pages/404'
import Storages from './pages/Storages'
import StorageCreateForm from './pages/Storages/StorageCreateForm'
import Files from './pages/Files'
import UploadFileTo from './pages/Files/UploadFileTo'
import StorageWorkers from './pages/StorageWorkers'
import StorageWorkerCreateForm from './pages/StorageWorkers/StorageWorkerCreateForm'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/register" element={<Register />} />
        <Route path="/" element={<BasicLayout />}>
          <Route index element={<Navigate to="/storages" replace />} />
          <Route path="storages" element={<Storages />} />
          <Route path="storages/register" element={<StorageCreateForm />} />
          <Route path="storages/:id/files/*" element={<Files />} />
          <Route path="storages/:id/upload_to" element={<UploadFileTo />} />
          <Route path="storage_workers" element={<StorageWorkers />} />
          <Route path="storage_workers/register" element={<StorageWorkerCreateForm />} />
          <Route path="*" element={<NotFound />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
