import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Box, Typography, TextField, Button, Stack,
  FormControl, InputLabel, Select, MenuItem,
} from '@mui/material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'
import { useApiAction } from '../../common/use_api_action'
import Panel from '../../components/Panel'

export default function StorageWorkerCreateForm() {
  const navigate = useNavigate()
  const run = useApiAction(useAlert())
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

  const handleSubmit = (e) => {
    e.preventDefault()
    return run(() => API.storageWorkers.create(name, token, storageId || null), {
      success: 'Worker created',
      onSuccess: () => navigate('/storage_workers'),
    })
  }

  return (
    <Box>
      <Typography variant="h5" sx={{ mb: 3 }}>Create Worker</Typography>
      <Panel sx={{ p: 3, maxWidth: 480 }}>
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
      </Panel>
    </Box>
  )
}
