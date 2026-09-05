import { normalizeUploadPath } from '../Files/upload_conflicts.js'

// Directories first, then by name, for the local filesystem browser.
export function sortLocalEntries(entries) {
  return (entries || []).slice().sort((a, b) => {
    if (a.is_file !== b.is_file) return a.is_file ? 1 : -1
    return a.name.localeCompare(b.name)
  })
}

export function localEntryKey(entry) {
  return entry.path || entry.name
}

// Path of filePath relative to basePath (both with slashes normalized).
export function relativeLocalPath(filePath, basePath) {
  const file = normalizeUploadPath(filePath)
  const base = normalizeUploadPath(basePath)
  if (!base) return file
  return file.startsWith(`${base}/`) ? file.slice(base.length + 1) : file
}

// Directory portion of a relative path (everything before the last slash).
export function dirOf(relativePath) {
  const idx = relativePath.lastIndexOf('/')
  return idx === -1 ? '' : relativePath.substring(0, idx)
}

// Destination directory for a file: the chosen destPath plus the relative
// directory the file lives in. The backend appends the filename itself.
export function buildDestDir(destPath, relativePath) {
  const relDir = dirOf(relativePath)
  const base = normalizeUploadPath(destPath)
  if (base && relDir) return `${base}/${relDir}`
  return base || relDir
}

// Recursively lists the files under dirPath using `browse(path)`.
// Directories that fail to list are skipped.
export async function collectLocalFiles(dirPath, browse) {
  const result = []
  try {
    const items = await browse(dirPath)
    for (const item of (items || [])) {
      if (item.is_file) {
        result.push(item)
      } else {
        result.push(...await collectLocalFiles(item.path, browse))
      }
    }
  } catch {
    // skip inaccessible directories
  }
  return result
}

// Turns the selected browser entries into batch items ({ local_path,
// dest_path }). Paths stay relative to browsePath so a selected directory
// keeps its own name under destPath (selecting "test" at "a/b/test" yields
// "test/file.txt", not "a/b/test/file.txt").
export async function buildLocalBatchItems(selectedEntries, browsePath, destPath, browse) {
  const items = []
  for (const entry of selectedEntries) {
    const files = entry.is_file ? [entry] : await collectLocalFiles(entry.path, browse)
    for (const file of files) {
      const rel = relativeLocalPath(file.path, browsePath)
      items.push({ local_path: file.path, dest_path: buildDestDir(destPath, rel) })
    }
  }
  return items
}
