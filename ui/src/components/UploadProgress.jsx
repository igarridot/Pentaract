import { Box, LinearProgress, Typography } from '@mui/material'
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

  const uploadPercent = calculatePercent(uploadedBytes, totalBytes)
  const verifyPercent = calculatePercent(verifiedChunks, verificationTotal)

  const speed = useTransferSpeed(uploadedBytes)
  const speedText = speed > 0 && isUploading ? `${convertSize(speed)}/s` : ''
  const workersText = workersStatus === 'waiting_rate_limit' ? 'Workers waiting (rate limit)' : 'Workers active'

  const uploadChunkText = totalChunks > 0 ? `${uploadedChunks}/${totalChunks} chunks` : ''
  const verifyChunkText = verificationTotal > 0 ? `${verifiedChunks}/${verificationTotal} verified` : ''

  const progressText = totalBytes > 0
    ? `${convertSize(uploadedBytes)} / ${convertSize(totalBytes)}`
    : 'Preparing...'

  const title = isError
    ? 'Failed'
    : status === 'done'
      ? 'Complete'
      : 'Uploading'

  const showVerifyBar = verificationTotal > 0 || isVerifying

  const verificationBar = showVerifyBar
    ? (
      <Box sx={{ mt: 0.75 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.25 }}>
          <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem' }}>
            Verification
          </Typography>
          {verificationTotal > 0 && (
            <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem' }}>
              {verifyChunkText} — {verifyPercent}%
            </Typography>
          )}
        </Box>
        <LinearProgress
          variant={verificationTotal > 0 ? 'determinate' : 'indeterminate'}
          value={verifyPercent}
          color={isError ? 'error' : verifyPercent >= 100 ? 'success' : 'secondary'}
          sx={{ width: '100%' }}
        />
      </Box>
    )
    : null

  return (
    <ProgressCard
      title={title}
      subtitle={filename}
      percent={uploadPercent}
      variant={totalBytes > 0 ? 'determinate' : 'indeterminate'}
      progressColor={isError ? 'error' : uploadPercent >= 100 && !isActive ? 'success' : 'primary'}
      isError={isError}
      showCancel={isActive}
      onCancel={onCancel}
      secondaryBar={verificationBar}
    >
      <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 0.75, flexDirection: { xs: 'column', sm: 'row' } }}>
        <Typography variant="caption" color="text.secondary">
          {progressText}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ textAlign: { xs: 'left', sm: 'right' } }}>
          {[workersText, isActive && uploadChunkText, isActive && speedText].filter(Boolean).join(' \u00b7 ')}
        </Typography>
      </Box>
    </ProgressCard>
  )
}
