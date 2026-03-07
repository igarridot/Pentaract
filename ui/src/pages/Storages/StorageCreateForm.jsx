import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Box, Typography, Paper, TextField, Button, Stack } from '@mui/material'
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
      <Typography variant="h5" gutterBottom>Create Storage</Typography>
      <Paper sx={{ p: 3, maxWidth: 500 }}>
        <form onSubmit={handleSubmit}>
          <Stack spacing={2}>
            <TextField
              fullWidth label="Name" value={name}
              onChange={(e) => setName(e.target.value)} required
            />
            <TextField
              fullWidth label="Telegram Chat ID" value={chatId}
              onChange={(e) => setChatId(e.target.value)} required
              type="number"
              helperText="The numeric ID of the Telegram channel"
            />
            <Button variant="contained" type="submit" disabled={!name || !chatId}>
              Create
            </Button>
          </Stack>
        </form>
      </Paper>
    </Box>
  )
}
