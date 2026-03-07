import { useEffect, useRef } from 'react'
import { Box, LinearProgress, Typography, Paper, IconButton } from '@mui/material'
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
    <Paper sx={{ p: 2, mb: 2 }} variant="outlined">
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Typography variant="body2" noWrap sx={{ flexGrow: 1 }}>
          {isError ? `Upload failed: ${filename}` : isActive ? `Uploading: ${filename}` : `Upload complete: ${filename}`}
        </Typography>
        {isActive && onCancel && (
          <IconButton size="small" onClick={onCancel} title="Cancel upload" sx={{ ml: 1 }}>
            <CloseIcon fontSize="small" />
          </IconButton>
        )}
      </Box>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.5 }}>
        <Box sx={{ flexGrow: 1 }}>
          <LinearProgress
            variant={totalBytes > 0 ? 'determinate' : 'indeterminate'}
            value={percent}
            color={isError ? 'error' : isActive ? 'primary' : 'success'}
          />
        </Box>
        <Typography variant="body2" color="text.secondary" sx={{ minWidth: 240, textAlign: 'right' }} noWrap>
          {progressText}{chunkText && isActive ? ` · ${chunkText}` : ''}{speedText && isActive ? ` · ${speedText}` : ''}
        </Typography>
      </Box>
    </Paper>
  )
}
