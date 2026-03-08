import { Box, LinearProgress, Typography, Button } from '@mui/material'
import { convertSize } from '../common/size_converter'
import { useTransferSpeed } from '../common/use_transfer_speed'

export default function DownloadProgress({ filename, totalBytes, downloadedBytes, totalChunks, downloadedChunks, status, workersStatus, onCancel }) {
  const percent = totalBytes > 0 ? Math.round((downloadedBytes / totalBytes) * 100) : 0
  const isActive = status === 'downloading'
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

  return (
    <Box
      sx={{
        width: '100%',
        maxWidth: '100%',
        boxSizing: 'border-box',
        mb: 2,
        p: 2,
        bgcolor: isError ? 'rgba(255,59,48,0.06)' : isCancelled ? 'rgba(255,152,0,0.08)' : 'background.paper',
        borderRadius: 3,
        border: '1px solid',
        borderColor: isError ? 'rgba(255,59,48,0.15)' : isCancelled ? 'rgba(255,152,0,0.2)' : 'divider',
        overflow: 'hidden',
      }}
    >
      <Typography variant="body2" noWrap sx={{ fontWeight: 500, minWidth: 0, mb: 1 }}>
        {isError ? 'Download failed' : isCancelled ? 'Download cancelled' : isActive ? 'Downloading' : 'Download complete'}
        <Typography
          component="span"
          variant="body2"
          color="text.secondary"
          sx={{ ml: 0.5, maxWidth: '100%', overflow: 'hidden', textOverflow: 'ellipsis' }}
        >
          {filename}
        </Typography>
      </Typography>

      <LinearProgress
        variant={totalBytes > 0 ? 'determinate' : 'indeterminate'}
        value={percent}
        color={isError ? 'error' : isCancelled ? 'warning' : isActive ? 'primary' : 'success'}
        sx={{ mb: 0.75, width: '100%' }}
      />

      <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 0.75, flexDirection: { xs: 'column', sm: 'row' }, alignItems: { xs: 'flex-start', sm: 'center' } }}>
        <Typography variant="caption" color="text.secondary">
          {progressText}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ textAlign: { xs: 'left', sm: 'right' } }}>
          {[workersText, chunkText, speedText].filter(Boolean).join(' \u00b7 ')}
        </Typography>
      </Box>
      {isActive && onCancel && (
        <Button size="small" color="warning" onClick={onCancel} sx={{ mt: 1 }}>
          Cancel download
        </Button>
      )}
    </Box>
  )
}
