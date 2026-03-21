import { Box, Typography } from '@mui/material'
import { convertSize } from '../common/size_converter'
import { calculatePercent } from '../common/progress'
import { useTransferSpeed } from '../common/use_transfer_speed'
import ProgressCard from './ProgressCard'

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

  return (
    <ProgressCard
      title={title}
      subtitle={filename}
      percent={percent}
      variant={isVerifying || totalBytes > 0 ? 'determinate' : 'indeterminate'}
      progressColor={isError ? 'error' : isActive ? 'primary' : 'success'}
      isError={isError}
      showCancel={isActive}
      onCancel={onCancel}
    >
      <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 0.75, flexDirection: { xs: 'column', sm: 'row' } }}>
        <Typography variant="caption" color="text.secondary">
          {progressText}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ textAlign: { xs: 'left', sm: 'right' } }}>
          {[workersText, isActive && chunkText, isActive && speedText].filter(Boolean).join(' \u00b7 ')}
        </Typography>
      </Box>
    </ProgressCard>
  )
}
