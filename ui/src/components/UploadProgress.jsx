import { Box, LinearProgress, Typography, IconButton } from '@mui/material'
import { Close as CloseIcon } from '@mui/icons-material'
import { convertSize } from '../common/size_converter'
import { calculatePercent } from '../common/progress'
import { useTransferSpeed } from '../common/use_transfer_speed'

export default function UploadProgress({
  filename,
  totalBytes,
  uploadedBytes,
  totalChunks,
  uploadedChunks,
  verificationTotal,
  verifiedChunks,
  status,
  workersStatus,
  onCancel,
}) {
  const isUploading = status === 'uploading'
  const isVerifying = status === 'verifying'
  const isActive = isUploading || isVerifying
  const isError = status === 'error'
  const percent = isVerifying
    ? calculatePercent(verifiedChunks, verificationTotal)
    : calculatePercent(uploadedBytes, totalBytes)
  const speed = useTransferSpeed(uploadedBytes)

  const speedText = speed > 0 && isUploading ? `${convertSize(speed)}/s` : ''
  const workersText = workersStatus === 'waiting_rate_limit' ? 'Workers waiting (rate limit)' : 'Workers active'
  const chunkText = isVerifying
    ? (verificationTotal > 0 ? `${verifiedChunks}/${verificationTotal} chunks verified` : 'Verifying chunks')
    : (totalChunks > 0 ? `${uploadedChunks}/${totalChunks} chunks` : '')

  const progressText = isVerifying && totalBytes > 0
    ? `${convertSize(uploadedBytes)} / ${convertSize(totalBytes)} uploaded`
    : totalBytes > 0
      ? `${convertSize(uploadedBytes)} / ${convertSize(totalBytes)}`
      : 'Preparing...'

  const title = isError
    ? 'Failed'
    : isVerifying
      ? 'Verifying'
      : isUploading
        ? 'Uploading'
        : 'Complete'

  const progressVariant = isVerifying || totalBytes > 0 ? 'determinate' : 'indeterminate'

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
        overflow: 'hidden',
      }}
    >
      <Box sx={{ display: 'flex', alignItems: { xs: 'flex-start', sm: 'center' }, justifyContent: 'space-between', mb: 1, minWidth: 0 }}>
        <Typography variant="body2" sx={{ fontWeight: 500, flexGrow: 1, minWidth: 0, pr: 1, wordBreak: 'break-word' }}>
          {title}
          <Typography
            component="span"
            variant="body2"
            color="text.secondary"
            sx={{ ml: 0.5, maxWidth: '100%', overflowWrap: 'anywhere' }}
          >
            {filename}
          </Typography>
        </Typography>
        {isActive && onCancel && (
          <IconButton size="small" onClick={onCancel} sx={{ ml: 1, opacity: 0.5, '&:hover': { opacity: 1 } }}>
            <CloseIcon sx={{ fontSize: 16 }} />
          </IconButton>
        )}
      </Box>
      <LinearProgress
        variant={progressVariant}
        value={percent}
        color={isError ? 'error' : isActive ? 'primary' : 'success'}
        sx={{ mb: 0.75, width: '100%' }}
      />
      <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 0.75, flexDirection: { xs: 'column', sm: 'row' } }}>
        <Typography variant="caption" color="text.secondary">
          {progressText}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ textAlign: { xs: 'left', sm: 'right' } }}>
          {[workersText, isActive && chunkText, isActive && speedText].filter(Boolean).join(' \u00b7 ')}
        </Typography>
      </Box>
    </Box>
  )
}
