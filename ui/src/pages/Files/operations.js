export function createBulkOperation(operation, total) {
  const bulkOperation = {
    operation,
    status: 'running',
    total,
    completed: 0,
    transferIds: [],
  }

  if (operation === 'upload' || operation === 'download') {
    bulkOperation.terminalStatuses = {}
    bulkOperation.startedAll = false
  }

  return bulkOperation
}

function getTrackedTransferStates(bulkOperation, uploadStates = [], downloadStates = []) {
  const transferIds = bulkOperation?.transferIds || []
  if (bulkOperation?.operation === 'upload') {
    return uploadStates.filter((state) => transferIds.includes(state.id))
  }
  if (bulkOperation?.operation === 'download') {
    return downloadStates.filter((state) => transferIds.includes(state.id))
  }
  return []
}

export function getBulkOperationMetrics(bulkOperation, uploadStates = [], downloadStates = []) {
  if (bulkOperation?.operation === 'move') {
    return {
      totalBytes: bulkOperation.total || 0,
      processedBytes: bulkOperation.completed || 0,
      totalChunks: bulkOperation.total || 0,
      processedChunks: bulkOperation.completed || 0,
      workersStatus: 'active',
    }
  }

  const transferStates = getTrackedTransferStates(bulkOperation, uploadStates, downloadStates)

  return {
    totalBytes: transferStates.reduce((sum, state) => sum + (state.totalBytes || 0), 0),
    processedBytes: transferStates.reduce((sum, state) => sum + ((state.uploadedBytes ?? state.downloadedBytes) || 0), 0),
    totalChunks: transferStates.reduce((sum, state) => sum + (state.totalChunks || 0), 0),
    processedChunks: transferStates.reduce((sum, state) => sum + ((state.uploadedChunks ?? state.downloadedChunks) || 0), 0),
    workersStatus: transferStates.some((state) => state.workersStatus === 'waiting_rate_limit') ? 'waiting_rate_limit' : 'active',
  }
}

async function settlePromise(promise) {
  if (!promise) return undefined
  try {
    return await promise
  } catch (error) {
    return error
  }
}

export async function runPhasedTransferPipeline(items, startTransfer, options = {}) {
  const maxActive = Math.max(1, options.maxActive ?? 2)
  const active = []
  let lastStarted = null

  for (const item of items) {
    if (lastStarted?.requestPromise) {
      await settlePromise(lastStarted.requestPromise)
    }

    while (active.length >= maxActive) {
      await settlePromise(active[0]?.completionPromise)
      active.shift()
    }

    const transfer = await startTransfer(item)
    if (!transfer) break

    active.push(transfer)
    lastStarted = transfer
  }

  if (lastStarted?.requestPromise) {
    await settlePromise(lastStarted.requestPromise)
  }

  await Promise.all(active.map((transfer) => settlePromise(transfer.completionPromise)))
}

export function getMediaType(name) {
  const ext = name?.split('.').pop()?.toLowerCase() || ''
  if (['jpg', 'jpeg', 'png', 'gif', 'webp', 'bmp', 'svg'].includes(ext)) return 'image'
  if (['mp4', 'webm', 'ogg', 'mov', 'm4v'].includes(ext)) return 'video'
  return null
}

export function getItemPath(item) {
  return `${item?.path || item?.name || ''}`.replace(/\/+$/, '')
}

export function buildRenamedPath(item, newName) {
  const sourcePath = getItemPath(item)
  const pathParts = sourcePath.split('/').filter(Boolean)
  if (pathParts.length === 0) return newName
  pathParts[pathParts.length - 1] = newName
  return pathParts.join('/')
}

export function buildBulkMoveTargetPath(targetPath, itemName) {
  return targetPath ? `${targetPath}/${itemName}` : itemName
}
