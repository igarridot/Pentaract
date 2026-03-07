import {
  Dialog, DialogTitle, DialogContent, DialogActions, Button, Typography,
} from '@mui/material'

export default function ActionConfirmDialog({ open, entity, action, description, onConfirm, onCancel }) {
  return (
    <Dialog open={open} onClose={onCancel} maxWidth="xs" fullWidth>
      <DialogTitle>{action} {entity}</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary">
          {description}
        </Typography>
      </DialogContent>
      <DialogActions>
        <Button onClick={onCancel} color="inherit">Cancel</Button>
        <Button onClick={onConfirm} color="error" variant="contained">
          {action}
        </Button>
      </DialogActions>
    </Dialog>
  )
}
