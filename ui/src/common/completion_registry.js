// Lets callers await the terminal status of an in-flight transfer (upload or
// download) that reports its outcome asynchronously over SSE.
export function createCompletionRegistry() {
  const pending = new Map()

  return {
    waitFor(transferId) {
      const existing = pending.get(transferId)
      if (existing) return existing.promise

      let resolvePromise
      const promise = new Promise((resolve) => {
        resolvePromise = resolve
      })

      pending.set(transferId, { promise, resolve: resolvePromise })
      return promise
    },

    settle(transferId, status = 'done') {
      const entry = pending.get(transferId)
      if (!entry) return false

      pending.delete(transferId)
      entry.resolve(status)
      return true
    },

    clear(status = 'cancelled') {
      for (const [transferId, entry] of pending.entries()) {
        pending.delete(transferId)
        entry.resolve(status)
      }
    },
  }
}
