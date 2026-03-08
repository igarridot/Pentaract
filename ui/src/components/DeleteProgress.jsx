import { Box, LinearProgress, Typography } from '@mui/material'

export default function DeleteProgress({ label, totalChunks, deletedChunks, status, workersStatus }) {
  const isActive = status === 'deleting'
  const isError = status === 'error'
  const percent = totalChunks > 0 ? Math.round((deletedChunks / totalChunks) * 100) : 0
  const pending = totalChunks > 0 ? Math.max(totalChunks - deletedChunks, 0) : 0
  const workersText = workersStatus === 'waiting_rate_limit' ? 'Workers waiting (rate limit)' : 'Workers active'

  return (
    <Box
      sx={{
        width: '100%',
        maxWidth: '100%',
        boxSizing: 'border-box',
        mb: 2,
        p: 2,
        bgcolor: isError ? 'rgba(255,59,48,0.06)' : 'background.paper',
        borderRadius: 3,
        border: '1px solid',
        borderColor: isError ? 'rgba(255,59,48,0.15)' : 'divider',
      }}
    >
      <Typography variant="body2" sx={{ fontWeight: 500, mb: 1 }}>
        {isError ? 'Delete failed' : isActive ? 'Deleting' : 'Delete complete'}
        <Typography component="span" variant="body2" color="text.secondary" sx={{ ml: 0.5 }}>
          {label}
        </Typography>
      </Typography>

      <LinearProgress
        variant={totalChunks > 0 ? 'determinate' : 'indeterminate'}
        value={percent}
        color={isError ? 'error' : isActive ? 'primary' : 'success'}
        sx={{ mb: 0.75, width: '100%' }}
      />

      <Typography variant="caption" color="text.secondary">
        {totalChunks > 0 ? `${workersText} · ${deletedChunks}/${totalChunks} chunks · ${pending} pending` : `${workersText} · Calculating chunks...`}
      </Typography>
    </Box>
  )
}
