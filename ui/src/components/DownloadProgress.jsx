import { useEffect, useRef } from 'react'
import { Box, LinearProgress, Typography } from '@mui/material'
import { convertSize } from '../common/size_converter'

export default function DownloadProgress({ filename, totalBytes, downloadedBytes, totalChunks, downloadedChunks, status }) {
  const percent = totalBytes > 0 ? Math.round((downloadedBytes / totalBytes) * 100) : 0
  const isActive = status === 'downloading'
  const isError = status === 'error'
  const pendingChunks = totalChunks > 0 ? Math.max(totalChunks - downloadedChunks, 0) : null

  const speedRef = useRef({ lastBytes: 0, lastTime: Date.now(), speed: 0 })

  useEffect(() => {
    const now = Date.now()
    const s = speedRef.current
    const elapsed = (now - s.lastTime) / 1000
    if (elapsed > 0.3 && downloadedBytes >= s.lastBytes) {
      s.speed = (downloadedBytes - s.lastBytes) / elapsed
      s.lastBytes = downloadedBytes
      s.lastTime = now
    }
  }, [downloadedBytes])

  const speed = speedRef.current.speed
  const speedText = speed > 0 && isActive ? `${convertSize(speed)}/s` : ''
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
        bgcolor: isError ? 'rgba(255,59,48,0.06)' : 'background.paper',
        borderRadius: 3,
        border: '1px solid',
        borderColor: isError ? 'rgba(255,59,48,0.15)' : 'divider',
        overflow: 'hidden',
      }}
    >
      <Typography variant="body2" noWrap sx={{ fontWeight: 500, minWidth: 0, mb: 1 }}>
        {isError ? 'Download failed' : isActive ? 'Downloading ZIP' : 'Download complete'}
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
        color={isError ? 'error' : isActive ? 'primary' : 'success'}
        sx={{ mb: 0.75, width: '100%' }}
      />

      <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 0.75, flexDirection: { xs: 'column', sm: 'row' } }}>
        <Typography variant="caption" color="text.secondary">
          {progressText}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ textAlign: { xs: 'left', sm: 'right' } }}>
          {[chunkText, speedText].filter(Boolean).join(' \u00b7 ')}
        </Typography>
      </Box>
    </Box>
  )
}
