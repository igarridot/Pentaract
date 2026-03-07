const units = ['bytes', 'KiB', 'MiB', 'GiB', 'TiB']

export function convertSize(bytes) {
  if (bytes === 0) return '0 bytes'

  let unitIndex = 0
  let size = bytes

  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex++
  }

  const formatted = unitIndex === 0 ? size.toString() : size.toFixed(size % 1 === 0 ? 0 : 1)
  return `${formatted} ${units[unitIndex]}`
}
