export function normalizeUploadPath(path) {
  return (path || '').replace(/^\/+|\/+$/g, '')
}

export function buildUploadEntries(files, currentPath) {
  const basePath = normalizeUploadPath(currentPath)
  return files.map((file) => {
    const relativePath = file.webkitRelativePath || file.name
    const relativeDir = relativePath.includes('/')
      ? relativePath.slice(0, relativePath.lastIndexOf('/'))
      : ''
    const targetPath = [basePath, relativeDir].filter(Boolean).join('/')
    return { file, targetPath, filename: file.name }
  })
}

export async function resolveUploadEntries(entries, hasConflict, askConflictDecision) {
  const resolved = []
  let applyForAllAction = null

  for (const entry of entries) {
    const conflict = await hasConflict(entry.targetPath, entry.filename)
    if (!conflict) {
      resolved.push(entry)
      continue
    }

    let action = applyForAllAction
    if (!action) {
      const decision = await askConflictDecision(entry.filename, entry.targetPath)
      action = decision.action
      if (decision.applyForAll) {
        applyForAllAction = action
      }
    }

    if (action === 'skip') {
      continue
    }

    resolved.push(entry)
  }

  return resolved
}

export function fileNameFromPath(path) {
  return (path || '').split('/').pop() || path || ''
}

// Set of file names in a directory listing (folders excluded).
export function fileNamesOf(items) {
  return new Set((items || []).filter((item) => item.is_file).map((item) => item.name))
}

export function uploadEntryKey(entry) {
  return `${entry.targetPath}::${entry.filename}`
}

// Entries the conflict resolution left out, so callers can tell the user.
export function findSkippedEntries(entries, resolved) {
  const kept = new Set(resolved.map(uploadEntryKey))
  return entries.filter((entry) => !kept.has(uploadEntryKey(entry)))
}
