import { useState, useCallback, useRef, useEffect } from 'react'
import API from '../api'
import { createOperationId } from './operation_id'
import {
  applyDeleteProgressUpdate,
  createDeleteProgressState,
  getDeleteProgressResetDelay,
} from './delete_progress'

// Tracks one delete operation (file, folder or storage) through its SSE
// progress stream and clears the card after the terminal state.
export function useDeleteProgress() {
  const [deleteState, setDeleteState] = useState(null)
  const cancelRef = useRef(null)

  const stopTracking = useCallback(() => {
    if (cancelRef.current) cancelRef.current()
    cancelRef.current = null
  }, [])

  useEffect(() => stopTracking, [stopTracking])

  const scheduleReset = useCallback((status) => {
    setTimeout(() => setDeleteState(null), getDeleteProgressResetDelay(status))
  }, [])

  // Runs `perform(deleteId)` while following its progress. `onTerminal` fires
  // when the server reports done or error. Errors thrown by perform mark the
  // card as failed and are rethrown for the caller to report.
  const runTrackedDelete = useCallback(async (label, perform, { onTerminal } = {}) => {
    const deleteId = createOperationId()
    stopTracking()
    setDeleteState(createDeleteProgressState(label))

    cancelRef.current = API.files.subscribeDeleteProgress(deleteId, (data) => {
      setDeleteState((prev) => applyDeleteProgressUpdate(prev, data))
      if (data.status === 'done' || data.status === 'error') {
        stopTracking()
        if (onTerminal) onTerminal(data.status)
        scheduleReset(data.status)
      }
    })

    try {
      return await perform(deleteId)
    } catch (err) {
      stopTracking()
      setDeleteState((prev) => (prev ? { ...prev, status: 'error' } : null))
      scheduleReset('error')
      throw err
    }
  }, [scheduleReset, stopTracking])

  return { deleteState, runTrackedDelete, stopTracking }
}
