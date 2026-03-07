import { Chip } from '@mui/material'

const accessTypeConfig = {
  a: { label: 'Admin', color: '#d32f2f' },
  w: { label: 'Can edit', color: '#ff8f00' },
  r: { label: 'Viewer', color: '#0288d1' },
}

export default function AccessTypeChip({ type }) {
  const config = accessTypeConfig[type] || { label: type, color: '#999' }

  return (
    <Chip
      label={config.label}
      size="small"
      sx={{ backgroundColor: config.color, color: 'white' }}
    />
  )
}
