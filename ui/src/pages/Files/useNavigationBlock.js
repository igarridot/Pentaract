import { useEffect } from 'react'
import { useBlocker } from 'react-router-dom'

export function useNavigationBlock({ hasActiveFileOperation, isDeleting, isBulkDelete }) {
  useEffect(() => {
    if (!hasActiveFileOperation) return
    const handler = (e) => {
      const isDeleteFlow = isDeleting || isBulkDelete
      const message = isDeleteFlow
        ? 'Operation in progress. Leaving will cancel it. If delete is interrupted, the file will be irrecoverable and orphaned data may remain in storage.'
        : 'Operation in progress. Leaving will cancel it.'
      e.preventDefault()
      e.returnValue = message
      return message
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [hasActiveFileOperation, isDeleting, isBulkDelete])

  const blocker = useBlocker(hasActiveFileOperation)

  return { blocker }
}
