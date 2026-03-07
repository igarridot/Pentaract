import {
  Dialog, DialogTitle, DialogContent, DialogContentText, DialogActions, Button,
} from '@mui/material'

export default function NavigationBlockDialog({ blocker }) {
  if (!blocker || blocker.state !== 'blocked') return null

  return (
    <Dialog open onClose={() => blocker.reset()}>
      <DialogTitle>Upload in progress</DialogTitle>
      <DialogContent>
        <DialogContentText sx={{ fontSize: '0.875rem' }}>
          A file upload is currently in progress. Leaving this page will interrupt it.
        </DialogContentText>
      </DialogContent>
      <DialogActions>
        <Button onClick={() => blocker.reset()} variant="contained">
          Stay
        </Button>
        <Button onClick={() => blocker.proceed()} color="error">
          Leave
        </Button>
      </DialogActions>
    </Dialog>
  )
}
