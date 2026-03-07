import {
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
  Paper, IconButton,
} from '@mui/material'
import { Edit as EditIcon, Delete as DeleteIcon } from '@mui/icons-material'
import AccessTypeChip from './AccessTypeChip'

export default function Access({ users, currentUserId, onEdit, onDelete }) {
  return (
    <TableContainer component={Paper}>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell>Email</TableCell>
            <TableCell>Access</TableCell>
            <TableCell align="right">Actions</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {users.map((user) => (
            <TableRow key={user.id}>
              <TableCell sx={{ fontSize: '0.875rem' }}>{user.email}</TableCell>
              <TableCell><AccessTypeChip type={user.access_type} /></TableCell>
              <TableCell align="right">
                <IconButton
                  size="small"
                  disabled={user.id === currentUserId}
                  onClick={() => onEdit(user)}
                >
                  <EditIcon sx={{ fontSize: 16 }} />
                </IconButton>
                <IconButton
                  size="small"
                  disabled={user.id === currentUserId}
                  onClick={() => onDelete(user)}
                >
                  <DeleteIcon sx={{ fontSize: 16 }} />
                </IconButton>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  )
}
