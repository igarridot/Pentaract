import { Box, LinearProgress, Typography, Paper } from '@mui/material'

export default function UploadProgress({ filename, total, uploaded, status }) {
  const percent = total > 0 ? Math.round((uploaded / total) * 100) : 0
  const isActive = status === 'uploading'

  return (
    <Paper sx={{ p: 2, mb: 2 }} variant="outlined">
      <Typography variant="body2" noWrap gutterBottom>
        {isActive ? `Uploading: ${filename}` : `Upload complete: ${filename}`}
      </Typography>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        <Box sx={{ flexGrow: 1 }}>
          <LinearProgress
            variant={total > 0 ? 'determinate' : 'indeterminate'}
            value={percent}
            color={isActive ? 'primary' : 'success'}
          />
        </Box>
        <Typography variant="body2" color="text.secondary" sx={{ minWidth: 60 }}>
          {total > 0 ? `${uploaded}/${total}` : 'Preparing...'}
        </Typography>
      </Box>
    </Paper>
  )
}
