import { Box, LinearProgress, Typography, Button } from '@mui/material'
import { convertSize } from '../common/size_converter'
import { useTransferSpeed } from '../common/use_transfer_speed'

export default function BulkOperationProgress({
  operation,
  status,
  total,
  completed,
  totalBytes = 0,
  processedBytes = 0,
  totalChunks = 0,
  processedChunks = 0,
  workersStatus = 'active',
  onCancel,
}) {
  const isActive = status === 'running'
  const isError = status === 'error'
  const isDone = status === 'done'
  const isCancelled = status === 'cancelled'
  const percent = total > 0 ? Math.round((completed / total) * 100) : 0
  const speed = useTransferSpeed(processedBytes)
  const workersText = workersStatus === 'waiting_rate_limit' ? 'Workers waiting (rate limit)' : 'Workers active'
  const chunkText = totalChunks > 0 ? `${processedChunks}/${totalChunks} chunks` : ''
  const speedText = speed > 0 && isActive ? `${convertSize(speed)}/s` : ''
  const bytesText = totalBytes > 0 ? `${convertSize(processedBytes)} / ${convertSize(totalBytes)}` : ''
  const title = isError
    ? `Bulk ${operation} failed`
    : isCancelled
      ? `Bulk ${operation} cancelled`
    : isDone
      ? `Bulk ${operation} complete`
      : `Bulk ${operation} in progress`

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
        {title}
      </Typography>
      <LinearProgress
        variant={total > 0 ? 'determinate' : 'indeterminate'}
        value={percent}
        color={isError ? 'error' : isCancelled ? 'warning' : isActive ? 'primary' : 'success'}
        sx={{ mb: 0.75, width: '100%' }}
      />
      <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 0.75, flexDirection: { xs: 'column', sm: 'row' } }}>
        <Typography variant="caption" color="text.secondary">
          {total > 0 ? `${completed}/${total} items` : 'Preparing...'}
          {bytesText ? ` · ${bytesText}` : ''}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ textAlign: { xs: 'left', sm: 'right' } }}>
          {[workersText, chunkText, speedText].filter(Boolean).join(' · ')}
        </Typography>
      </Box>
      {isActive && onCancel && (
        <Button size="small" color="warning" onClick={onCancel} sx={{ mt: 1 }}>
          Cancel {operation}
        </Button>
      )}
    </Box>
  )
}
