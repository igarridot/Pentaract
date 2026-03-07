import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Box, Typography, TextField, Button, Stack } from '@mui/material'
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
      <Typography variant="h5" sx={{ mb: 3 }}>Upload File</Typography>
      <Box sx={{
        bgcolor: 'white',
        borderRadius: 3,
        border: '1px solid rgba(0,0,0,0.06)',
        p: 3,
        maxWidth: 480,
      }}>
        <form onSubmit={handleSubmit}>
          <Stack spacing={2.5}>
            <TextField
              fullWidth placeholder="Path (optional)" value={path}
              onChange={(e) => setPath(e.target.value)}
              helperText="e.g. folder1/subfolder"
            />
            <Button variant="outlined" component="label" sx={{ justifyContent: 'flex-start' }}>
              {file ? file.name : 'Choose file'}
              <input type="file" hidden onChange={(e) => setFile(e.target.files[0])} />
            </Button>
            <Button variant="contained" type="submit" disabled={!file}>
              Upload
            </Button>
          </Stack>
        </form>
      </Box>
    </Box>
  )
}
