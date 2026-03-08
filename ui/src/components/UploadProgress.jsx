import { useEffect, useRef } from 'react'
import { Box, LinearProgress, Typography, IconButton } from '@mui/material'
import { Close as CloseIcon } from '@mui/icons-material'
import { convertSize } from '../common/size_converter'

export default function UploadProgress({ filename, totalBytes, uploadedBytes, totalChunks, uploadedChunks, status, onCancel }) {
  const percent = totalBytes > 0 ? Math.round((uploadedBytes / totalBytes) * 100) : 0
  const isActive = status === 'uploading'
  const isError = status === 'error'

  const speedRef = useRef({ lastBytes: 0, lastTime: Date.now(), speed: 0 })

  useEffect(() => {
    const now = Date.now()
    const s = speedRef.current
    const elapsed = (now - s.lastTime) / 1000
    if (elapsed > 0.3 && uploadedBytes > s.lastBytes) {
      s.speed = (uploadedBytes - s.lastBytes) / elapsed
      s.lastBytes = uploadedBytes
      s.lastTime = now
    }
  }, [uploadedBytes])

  const speed = speedRef.current.speed
  const speedText = speed > 0 ? `${convertSize(speed)}/s` : ''

  const chunkText = totalChunks > 0
    ? `${uploadedChunks}/${totalChunks} chunks`
    : ''

  const progressText = totalBytes > 0
    ? `${convertSize(uploadedBytes)} / ${convertSize(totalBytes)}`
    : 'Preparing...'

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
          {isError ? `Failed` : isActive ? `Uploading` : `Complete`}
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
        variant={totalBytes > 0 ? 'determinate' : 'indeterminate'}
        value={percent}
        color={isError ? 'error' : isActive ? 'primary' : 'success'}
        sx={{ mb: 0.75, width: '100%' }}
      />
      <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 0.75, flexDirection: { xs: 'column', sm: 'row' } }}>
        <Typography variant="caption" color="text.secondary">
          {progressText}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ textAlign: { xs: 'left', sm: 'right' } }}>
          {[chunkText && isActive ? chunkText : '', speedText && isActive ? speedText : ''].filter(Boolean).join(' \u00b7 ')}
        </Typography>
      </Box>
    </Box>
  )
}
