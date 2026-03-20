import { normalizeUploadPath } from '../Files/upload_conflicts.js'

export function normalizeLocalPath(rawPath = '') {
  return String(rawPath || '')
    .replace(/\\/g, '/')
    .replace(/^\/+|\/+$/g, '')
    .replace(/\/+/g, '/')
}

export function buildLocalUploadEntries(files, currentPath, destinationPath) {
  const normalizedCurrentPath = normalizeLocalPath(currentPath)
  const normalizedDestinationPath = normalizeUploadPath(destinationPath)

  return (files || []).map((file) => {
    const sourcePath = normalizeLocalPath(file.path)
    const relativePath = normalizedCurrentPath && sourcePath.startsWith(`${normalizedCurrentPath}/`)
      ? sourcePath.slice(normalizedCurrentPath.length + 1)
      : sourcePath
    const segments = relativePath.split('/').filter(Boolean)
    const filename = segments[segments.length - 1] || file.name
    const relativeDir = segments.slice(0, -1).join('/')
    const targetPath = normalizeUploadPath([normalizedDestinationPath, relativeDir].filter(Boolean).join('/'))

    return {
      sourcePath,
      filename,
      size: file.size || 0,
      targetPath,
    }
  })
}
