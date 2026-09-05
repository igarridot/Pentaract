export function createUploadCompletionRegistry() {
  const pending = new Map()

  return {
    waitFor(uploadId) {
      const existing = pending.get(uploadId)
      if (existing) return existing.promise

      let resolvePromise
      const promise = new Promise((resolve) => {
        resolvePromise = resolve
      })

      pending.set(uploadId, { promise, resolve: resolvePromise })
      return promise
    },

    settle(uploadId, status = 'done') {
      const entry = pending.get(uploadId)
      if (!entry) return false

      pending.delete(uploadId)
      entry.resolve(status)
      return true
    },

    clear(status = 'cancelled') {
      for (const [uploadId, entry] of pending.entries()) {
        pending.delete(uploadId)
        entry.resolve(status)
      }
    },
  }
}
