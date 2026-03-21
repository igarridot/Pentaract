import { useState, useEffect, useCallback } from 'react'
import { useParams, useLocation } from 'react-router-dom'
import API from '../../api'

export function useFileNavigation(addAlert) {
  const { id: storageId } = useParams()
  const location = useLocation()

  const prefix = `/storages/${storageId}/files/`
  const currentPathFromUrl = location.pathname.startsWith(prefix)
    ? location.pathname.slice(prefix.length)
    : ''
  let currentPath = currentPathFromUrl
  try {
    currentPath = decodeURIComponent(currentPathFromUrl)
  } catch {
    currentPath = currentPathFromUrl
  }

  const [items, setItems] = useState([])
  const [search, setSearch] = useState('')
  const [searchResults, setSearchResults] = useState(null)

  const loadTree = useCallback(async () => {
    try {
      const data = await API.files.tree(storageId, currentPath)
      setItems(data || [])
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }, [storageId, currentPath])

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
    try {
      const data = await API.files.search(storageId, currentPath, search)
      setSearchResults(data || [])
    } catch (err) {
      addAlert(err.message, 'error')
    }
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
