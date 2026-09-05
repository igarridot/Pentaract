import { useState, useEffect, useCallback } from 'react'
import { useParams, useLocation } from 'react-router-dom'
import API from '../../api'
import { useApiAction } from '../../common/use_api_action'

function decodePath(path) {
  try {
    return decodeURIComponent(path)
  } catch {
    return path
  }
}

export function useFileNavigation(addAlert) {
  const { id: storageId } = useParams()
  const location = useLocation()
  const run = useApiAction(addAlert)

  const prefix = `/storages/${storageId}/files/`
  const currentPath = decodePath(location.pathname.startsWith(prefix) ? location.pathname.slice(prefix.length) : '')

  const [items, setItems] = useState([])
  const [search, setSearch] = useState('')
  const [searchResults, setSearchResults] = useState(null)

  const loadTree = useCallback(() => (
    run(() => API.files.tree(storageId, currentPath), { onSuccess: (data) => setItems(data || []) })
  ), [run, storageId, currentPath])

  useEffect(() => {
    loadTree()
    setSearchResults(null)
    setSearch('')
  }, [loadTree])

  const handleSearch = async (e) => {
    e.preventDefault()
    if (!search) {
      setSearchResults(null)
      return
    }
    await run(() => API.files.search(storageId, currentPath, search), { onSuccess: (data) => setSearchResults(data || []) })
  }

  const pathParts = currentPath.split('/').filter(Boolean)

  return {
    storageId,
    prefix,
    currentPath,
    pathParts,
    items,
    setItems,
    search,
    setSearch,
    searchResults,
    setSearchResults,
    loadTree,
    handleSearch,
  }
}
