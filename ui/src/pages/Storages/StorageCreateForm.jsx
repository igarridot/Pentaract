import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Box, Typography, TextField, Button, Stack } from '@mui/material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'

export default function StorageCreateForm() {
  const navigate = useNavigate()
  const addAlert = useAlert()
  const [name, setName] = useState('')
  const [chatId, setChatId] = useState('')

  const handleSubmit = async (e) => {
    e.preventDefault()
    try {
      await API.storages.create(name, parseInt(chatId, 10))
      addAlert('Storage created', 'success')
      navigate('/storages')
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  return (
    <Box>
      <Typography variant="h5" sx={{ mb: 3 }}>Create Storage</Typography>
      <Box sx={{
        bgcolor: 'background.paper',
        borderRadius: 3,
        border: '1px solid',
        borderColor: 'divider',
        p: 3,
        maxWidth: 480,
      }}>
        <form onSubmit={handleSubmit}>
          <Stack spacing={2.5}>
            <TextField
              fullWidth placeholder="Storage name" value={name}
              onChange={(e) => setName(e.target.value)} required
            />
            <TextField
              fullWidth placeholder="Telegram Chat ID" value={chatId}
              onChange={(e) => setChatId(e.target.value)} required
              type="number"
              helperText="The numeric ID of the Telegram channel"
            />
            <Button variant="contained" type="submit" disabled={!name || !chatId}>
              Create Storage
            </Button>
          </Stack>
        </form>
      </Box>
    </Box>
  )
}
