import { useState } from 'react'
import API from '../../api'
import { useApiAction } from '../../common/use_api_action'
import { useDeleteProgress } from '../../common/use_delete_progress'
import { getItemPath } from './operations'

export function useDeleteOperation(addAlert, storageId, loadTree) {
  const run = useApiAction(addAlert)
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [forceDelete, setForceDelete] = useState(false)
  const { deleteState, runTrackedDelete, stopTracking } = useDeleteProgress()

  const isDeleting = deleteState?.status === 'deleting'

  const confirmDelete = async () => {
    const target = deleteTarget
    if (!target) return

    setDeleteTarget(null)
    const path = getItemPath(target)
    await run(
      () => runTrackedDelete(
        target.name || path,
        (deleteId) => API.files.delete(storageId, path, deleteId, forceDelete),
        { onTerminal: (status) => { if (status === 'done') loadTree() } },
      ),
      { success: 'Deleted' },
    )
    setForceDelete(false)
    loadTree()
  }

  return {
    deleteTarget,
    setDeleteTarget,
    forceDelete,
    setForceDelete,
    deleteState,
    isDeleting,
    confirmDelete,
    cleanupDelete: stopTracking,
  }
}
