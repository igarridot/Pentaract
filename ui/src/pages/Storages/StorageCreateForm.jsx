import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Box, Typography, TextField, Button, Stack } from '@mui/material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'
import { useApiAction } from '../../common/use_api_action'
import Panel from '../../components/Panel'

export default function StorageCreateForm() {
  const navigate = useNavigate()
  const run = useApiAction(useAlert())
  const [name, setName] = useState('')
  const [chatId, setChatId] = useState('')

  const handleSubmit = (e) => {
    e.preventDefault()
    return run(() => API.storages.create(name, parseInt(chatId, 10)), {
      success: 'Storage created',
      onSuccess: () => navigate('/storages'),
    })
  }

  return (
    <Box>
      <Typography variant="h5" sx={{ mb: 3 }}>Create Storage</Typography>
      <Panel sx={{ p: 3, maxWidth: 480 }}>
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
      </Panel>
    </Box>
  )
}
