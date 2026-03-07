import {
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
  Paper, IconButton,
} from '@mui/material'
import { Edit as EditIcon, Delete as DeleteIcon } from '@mui/icons-material'
import AccessTypeChip from './AccessTypeChip'

export default function Access({ users, currentUserId, onEdit, onDelete }) {
  return (
    <TableContainer component={Paper} variant="outlined">
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
              <TableCell>{user.email}</TableCell>
              <TableCell><AccessTypeChip type={user.access_type} /></TableCell>
              <TableCell align="right">
                <IconButton
                  size="small"
                  disabled={user.id === currentUserId}
                  onClick={() => onEdit(user)}
                >
                  <EditIcon fontSize="small" />
                </IconButton>
                <IconButton
                  size="small"
                  disabled={user.id === currentUserId}
                  onClick={() => onDelete(user)}
                >
                  <DeleteIcon fontSize="small" />
                </IconButton>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  )
}
