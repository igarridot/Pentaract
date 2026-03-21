import { Box, Typography } from '@mui/material'
import { convertSize } from '../common/size_converter'
import { calculatePercent } from '../common/progress'
import { useTransferSpeed } from '../common/use_transfer_speed'
import ProgressCard from './ProgressCard'

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
  const isCancelled = status === 'cancelled'
  const isDone = status === 'done'
  const percent = calculatePercent(completed, total)
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
    <ProgressCard
      title={title}
      percent={percent}
      variant={total > 0 ? 'determinate' : 'indeterminate'}
      progressColor={isError ? 'error' : isCancelled ? 'warning' : isActive ? 'primary' : 'success'}
      isError={isError}
      showCancel={isActive}
      onCancel={onCancel}
      cancelLabel={`Cancel ${operation}`}
    >
      <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 0.75, flexDirection: { xs: 'column', sm: 'row' } }}>
        <Typography variant="caption" color="text.secondary">
          {total > 0 ? `${completed}/${total} items` : 'Preparing...'}
          {bytesText ? ` · ${bytesText}` : ''}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ textAlign: { xs: 'left', sm: 'right' } }}>
          {[workersText, chunkText, speedText].filter(Boolean).join(' · ')}
        </Typography>
      </Box>
    </ProgressCard>
  )
}
