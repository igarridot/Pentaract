import {
  Dialog, DialogTitle, DialogContent, DialogActions, Button, Typography,
} from '@mui/material'

export default function ActionConfirmDialog({ open, entity, action, description, onConfirm, onCancel }) {
  return (
    <Dialog open={open} onClose={onCancel} maxWidth="xs" fullWidth>
      <DialogTitle>{action} {entity}</DialogTitle>
      <DialogContent>
        <Typography>{description}</Typography>
      </DialogContent>
      <DialogActions>
        <Button onClick={onCancel}>Cancel</Button>
        <Button onClick={onConfirm} color="warning" variant="contained">
          {action}
        </Button>
      </DialogActions>
    </Dialog>
  )
}
