import { Box, Typography } from '@mui/material'
import { convertSize } from '../common/size_converter'
import { calculatePercent } from '../common/progress'
import { useTransferSpeed } from '../common/use_transfer_speed'
import ProgressCard from './ProgressCard'

export default function DownloadProgress({ filename, totalBytes, downloadedBytes, totalChunks, downloadedChunks, status, workersStatus, errorMessage, onCancel }) {
  const percent = calculatePercent(downloadedBytes, totalBytes)
  const isActive = status === 'downloading'
  const isInterrupted = status === 'interrupted'
  const isError = status === 'error'
  const isCancelled = status === 'cancelled'
  const pendingChunks = totalChunks > 0 ? Math.max(totalChunks - downloadedChunks, 0) : null
  const speed = useTransferSpeed(downloadedBytes)

  const speedText = speed > 0 && isActive ? `${convertSize(speed)}/s` : ''
  const workersText = workersStatus === 'waiting_rate_limit' ? 'Workers waiting (rate limit)' : 'Workers active'
  const chunkText = totalChunks > 0
    ? `${downloadedChunks}/${totalChunks} chunks \u00b7 ${pendingChunks} pending`
    : `${downloadedChunks} chunks`
  const progressText = totalBytes > 0
    ? `${convertSize(downloadedBytes)} / ${convertSize(totalBytes)}`
    : convertSize(downloadedBytes)

  const title = isError
    ? 'Download failed'
    : isCancelled
      ? 'Download cancelled'
      : isInterrupted
        ? 'Waiting for the browser'
        : isActive
          ? 'Downloading'
          : 'Download complete'

  return (
    <ProgressCard
      title={title}
      subtitle={filename}
      percent={percent}
      variant={totalBytes > 0 || isInterrupted ? 'determinate' : 'indeterminate'}
      progressColor={isError ? 'error' : isCancelled || isInterrupted ? 'warning' : isActive ? 'primary' : 'success'}
      isError={isError}
      isWarning={isCancelled || isInterrupted}
      showCancel={isActive || isInterrupted}
      onCancel={onCancel}
      cancelLabel="Cancel download"
      afterBar={
        isError && errorMessage ? (
          <Typography variant="caption" color="error.main" sx={{ display: 'block', mt: 1 }}>
            {errorMessage}
          </Typography>
        ) : isInterrupted ? (
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
            The browser paused this download. If it shows an insecure download warning, choose to keep the file and the download will continue here.
          </Typography>
        ) : null
      }
    >
      <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 0.75, flexDirection: { xs: 'column', sm: 'row' }, alignItems: { xs: 'flex-start', sm: 'center' } }}>
        <Typography variant="caption" color="text.secondary">
          {progressText}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ textAlign: { xs: 'left', sm: 'right' } }}>
          {[workersText, chunkText, speedText].filter(Boolean).join(' \u00b7 ')}
        </Typography>
      </Box>
    </ProgressCard>
  )
}
