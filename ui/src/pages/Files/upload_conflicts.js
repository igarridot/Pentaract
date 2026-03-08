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
