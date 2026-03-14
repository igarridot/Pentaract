export function calculatePercent(processed, total) {
  if (!Number.isFinite(processed) || !Number.isFinite(total) || total <= 0) return 0
  const bounded = Math.min(Math.max(processed, 0), total)
  return Math.round((bounded / total) * 100)
}

export function isTerminalTransferStatus(status) {
  return status === 'done' || status === 'skipped' || status === 'error' || status === 'cancelled'
}

export function summarizeTerminalStatuses(terminalStatuses = {}) {
  const summary = {
    completed: 0,
    doneCount: 0,
    skippedCount: 0,
    errorCount: 0,
    cancelledCount: 0,
  }

  Object.values(terminalStatuses).forEach((status) => {
    if (!isTerminalTransferStatus(status)) return
    summary.completed += 1
    if (status === 'done') summary.doneCount += 1
    if (status === 'skipped') summary.skippedCount += 1
    if (status === 'error') summary.errorCount += 1
    if (status === 'cancelled') summary.cancelledCount += 1
  })

  return summary
}

export function resolveBulkTransferStatus(summary) {
  if (summary.errorCount > 0) return 'error'
  if (summary.cancelledCount > 0) return 'cancelled'
  return 'done'
}
