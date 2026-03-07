import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Box, Typography, TextField, Button, Stack,
  FormControl, InputLabel, Select, MenuItem,
} from '@mui/material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'

export default function StorageWorkerCreateForm() {
  const navigate = useNavigate()
  const addAlert = useAlert()
  const [name, setName] = useState('')
  const [token, setToken] = useState('')
  const [storageId, setStorageId] = useState('')
  const [storages, setStorages] = useState([])

  useEffect(() => {
    const load = async () => {
      try {
        const data = await API.storages.list()
        setStorages(data || [])
      } catch {}
    }
    load()
  }, [])

  const handleSubmit = async (e) => {
    e.preventDefault()
    try {
      await API.storageWorkers.create(name, token, storageId || null)
      addAlert('Worker created', 'success')
      navigate('/storage_workers')
    } catch (err) {
      addAlert(err.message, 'error')
    }
  }

  return (
    <Box>
      <Typography variant="h5" sx={{ mb: 3 }}>Create Worker</Typography>
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
              fullWidth placeholder="Worker name" value={name}
              onChange={(e) => setName(e.target.value)} required
            />
            <TextField
              fullWidth placeholder="Telegram Bot Token" value={token}
              onChange={(e) => setToken(e.target.value)} required
              helperText="Get this from @BotFather on Telegram"
            />
            <FormControl fullWidth>
              <InputLabel>Storage (optional)</InputLabel>
              <Select
                value={storageId}
                onChange={(e) => setStorageId(e.target.value)}
                label="Storage (optional)"
              >
                <MenuItem value="">None</MenuItem>
                {storages.map((s) => (
                  <MenuItem key={s.id} value={s.id}>{s.name}</MenuItem>
                ))}
              </Select>
            </FormControl>
            <Button variant="contained" type="submit" disabled={!name || !token}>
              Create Worker
            </Button>
          </Stack>
        </form>
      </Box>
    </Box>
  )
}
