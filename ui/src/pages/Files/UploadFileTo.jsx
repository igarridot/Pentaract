import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Box, Typography, Paper, TextField, Button, Stack } from '@mui/material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'

export default function UploadFileTo() {
  const { id: storageId } = useParams()
  const navigate = useNavigate()
  const addAlert = useAlert()
  const [path, setPath] = useState('')
  const [file, setFile] = useState(null)

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!file) return

    try {
      await API.files.uploadTo(storageId, path, file)
      addAlert('File uploaded', 'success')
      navigate(`/storages/${storageId}/files/${path}`)
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  return (
    <Box>
      <Typography variant="h5" gutterBottom>Upload File</Typography>
      <Paper sx={{ p: 3, maxWidth: 500 }}>
        <form onSubmit={handleSubmit}>
          <Stack spacing={2}>
            <TextField
              fullWidth label="Path (optional)" value={path}
              onChange={(e) => setPath(e.target.value)}
              helperText="e.g. folder1/subfolder"
            />
            <Button variant="outlined" component="label">
              {file ? file.name : 'Choose file'}
              <input type="file" hidden onChange={(e) => setFile(e.target.files[0])} />
            </Button>
            <Button variant="contained" type="submit" disabled={!file}>
              Upload
            </Button>
          </Stack>
        </form>
      </Paper>
    </Box>
  )
}
