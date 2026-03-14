export const DELETE_PROGRESS_SUCCESS_DELAY_MS = 1500
export const DELETE_PROGRESS_ERROR_DELAY_MS = 3000

export function createDeleteProgressState(label) {
  return {
    label: label || 'item',
    totalChunks: 0,
    deletedChunks: 0,
    status: 'deleting',
    workersStatus: 'active',
  }
}

export function applyDeleteProgressUpdate(previous, data) {
  return {
    ...previous,
    totalChunks: data.total ?? previous?.totalChunks ?? 0,
    deletedChunks: data.deleted ?? 0,
    status: data.status,
    workersStatus: data.workers_status ?? previous?.workersStatus ?? 'active',
  }
}

export function getDeleteProgressResetDelay(status) {
  return status === 'error' ? DELETE_PROGRESS_ERROR_DELAY_MS : DELETE_PROGRESS_SUCCESS_DELAY_MS
}
