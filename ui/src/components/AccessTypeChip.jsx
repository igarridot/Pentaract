import { Chip } from '@mui/material'

const accessTypeConfig = {
  a: { label: 'Admin', color: 'rgba(255,59,48,0.12)', textColor: '#ff3b30' },
  w: { label: 'Can edit', color: 'rgba(255,159,10,0.12)', textColor: '#ff9f0a' },
  r: { label: 'Viewer', color: 'rgba(0,113,227,0.12)', textColor: '#0071e3' },
}

export default function AccessTypeChip({ type }) {
  const config = accessTypeConfig[type] || { label: type, color: 'rgba(0,0,0,0.06)', textColor: '#86868b' }

  return (
    <Chip
      label={config.label}
      size="small"
      sx={{
        backgroundColor: config.color,
        color: config.textColor,
        fontWeight: 500,
        fontSize: '0.75rem',
      }}
    />
  )
}
