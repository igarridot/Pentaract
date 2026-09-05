import { useState, useEffect, useCallback } from 'react'

// Multi-select over the files currently listed. The selection is dropped when
// resetKey changes (a directory change) and pruned to the visible files.
export function useFileSelection(items, resetKey) {
  const [selectedFilePaths, setSelectedFilePaths] = useState([])

  useEffect(() => {
    setSelectedFilePaths([])
  }, [resetKey])

  const selectableFiles = items.filter((item) => item.is_file)

  useEffect(() => {
    const visiblePaths = new Set(selectableFiles.map((item) => item.path))
    setSelectedFilePaths((prev) => {
      const next = prev.filter((path) => visiblePaths.has(path))
      return next.length === prev.length ? prev : next
    })
  }, [items]) // eslint-disable-line react-hooks/exhaustive-deps -- selectableFiles derives from items

  const selectedFiles = selectableFiles.filter((item) => selectedFilePaths.includes(item.path))
  const allFilesSelected = selectableFiles.length > 0 && selectedFiles.length === selectableFiles.length

  const clearSelection = useCallback(() => setSelectedFilePaths([]), [])

  const toggleFileSelection = (item) => {
    if (!item?.is_file || !item.path) return
    setSelectedFilePaths((prev) => (
      prev.includes(item.path) ? prev.filter((path) => path !== item.path) : [...prev, item.path]
    ))
  }

  const toggleSelectAllFiles = () => {
    setSelectedFilePaths(allFilesSelected ? [] : selectableFiles.map((item) => item.path))
  }

  return {
    selectedFilePaths,
    selectableFiles,
    selectedFiles,
    allFilesSelected,
    clearSelection,
    toggleFileSelection,
    toggleSelectAllFiles,
  }
}
