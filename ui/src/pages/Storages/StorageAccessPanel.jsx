import { useState, useEffect, useCallback } from 'react'
import { Box, Typography, IconButton } from '@mui/material'
import { Add as AddIcon, Close as CloseIcon } from '@mui/icons-material'
import API from '../../api'
import { useAlert } from '../../components/AlertStack'
import { useApiAction } from '../../common/use_api_action'
import { getCurrentUserId } from '../../common/auth_guard'
import Access from '../../components/Access'
import GrantAccess from '../../components/GrantAccess'

// Lists who can use a storage and lets an admin grant, change or revoke access.
export default function StorageAccessPanel({ storageId, onClose }) {
  const addAlert = useAlert()
  const run = useApiAction(addAlert)
  const [users, setUsers] = useState([])
  const [grantOpen, setGrantOpen] = useState(false)
  const [candidates, setCandidates] = useState([])
  const [editUser, setEditUser] = useState(null)
  const currentUserId = getCurrentUserId()

  const load = useCallback(() => (
    run(() => API.access.list(storageId), { onSuccess: (data) => setUsers(data || []) })
  ), [run, storageId])

  useEffect(() => { load() }, [load])

  const openGrantDialog = () => run(() => API.access.candidates(storageId), {
    onSuccess: (data) => {
      setCandidates(data || [])
      setEditUser(null)
      setGrantOpen(true)
    },
  })

  const handleGrant = (email, accessType) => run(
    () => API.access.grant(storageId, email, accessType),
    { success: 'Access granted', onSuccess: load },
  )

  const handleRevoke = (user) => run(
    () => API.access.revoke(storageId, user.id),
    { success: 'Access revoked', onSuccess: load },
  )

  return (
    <Box sx={{ mt: 3 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1.5 }}>
        <Typography variant="h6" sx={{ fontSize: '1rem' }}>Access Control</Typography>
        <Box>
          <IconButton size="small" onClick={openGrantDialog} title="Grant access">
            <AddIcon sx={{ fontSize: 18 }} />
          </IconButton>
          <IconButton size="small" onClick={onClose} title="Close">
            <CloseIcon sx={{ fontSize: 18 }} />
          </IconButton>
        </Box>
      </Box>
      <Access
        users={users}
        currentUserId={currentUserId}
        onEdit={(user) => { setEditUser(user); setGrantOpen(true) }}
        onDelete={handleRevoke}
      />
      <GrantAccess
        open={grantOpen}
        onClose={() => { setGrantOpen(false); setEditUser(null); setCandidates([]) }}
        onGrant={handleGrant}
        editUser={editUser}
        candidates={candidates}
      />
    </Box>
  )
}
