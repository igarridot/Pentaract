import { useState, useCallback, useRef } from 'react'
import API from '../../api'
import { createOperationId } from '../../common/operation_id'
import {
  applyDeleteProgressUpdate,
  createDeleteProgressState,
  getDeleteProgressResetDelay,
} from '../../common/delete_progress'
import { getItemPath } from './operations'

export function useDeleteOperation(addAlert, storageId, loadTree) {
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [forceDelete, setForceDelete] = useState(false)
  const [deleteState, setDeleteState] = useState(null)
  const cancelDeleteProgressRef = useRef(null)

  const isDeleting = deleteState?.status === 'deleting'

  const confirmDelete = async () => {
    const target = deleteTarget
    if (!target) return

    setDeleteTarget(null)

    try {
      const path = getItemPath(target)
      const deleteId = createOperationId()
      if (cancelDeleteProgressRef.current) cancelDeleteProgressRef.current()
      setDeleteState(createDeleteProgressState(target.name || path))

      const cancel = API.files.subscribeDeleteProgress(deleteId, (data) => {
        setDeleteState((prev) => applyDeleteProgressUpdate(prev, data))

        if (data.status === 'done') {
          cancel()
          cancelDeleteProgressRef.current = null
          loadTree()
          setTimeout(() => setDeleteState(null), getDeleteProgressResetDelay(data.status))
        }
        if (data.status === 'error') {
          cancel()
          cancelDeleteProgressRef.current = null
          setTimeout(() => setDeleteState(null), getDeleteProgressResetDelay(data.status))
        }
      })
      cancelDeleteProgressRef.current = cancel

      await API.files.delete(storageId, path, deleteId, forceDelete)
      addAlert('Deleted', 'success')
      loadTree()
    } catch (err) {
      if (cancelDeleteProgressRef.current) {
        cancelDeleteProgressRef.current()
        cancelDeleteProgressRef.current = null
      }
      setDeleteState((prev) => (prev ? { ...prev, status: 'error' } : null))
      setTimeout(() => setDeleteState(null), getDeleteProgressResetDelay('error'))
      addAlert(err.message, 'error')
    } finally {
      setForceDelete(false)
      loadTree()
    }
  }

  const cleanupDelete = useCallback(() => {
    if (cancelDeleteProgressRef.current) cancelDeleteProgressRef.current()
  }, [])

  return {
    deleteTarget,
    setDeleteTarget,
    forceDelete,
    setForceDelete,
    deleteState,
    isDeleting,
    confirmDelete,
    cleanupDelete,
  }
}
