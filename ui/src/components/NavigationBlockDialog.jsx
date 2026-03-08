import {
  Dialog, DialogTitle, DialogContent, DialogContentText, DialogActions, Button,
} from '@mui/material'

export default function NavigationBlockDialog({
  blocker,
  isUploading = false,
  isDownloading = false,
  isDeleting = false,
  isMoving = false,
}) {
  if (!blocker || blocker.state !== 'blocked') return null

  const activeOps = [
    isUploading ? 'upload' : null,
    isDownloading ? 'download' : null,
    isDeleting ? 'delete' : null,
    isMoving ? 'move' : null,
  ].filter(Boolean)

  const opText = activeOps.length > 1
    ? `${activeOps.slice(0, -1).join(', ')} and ${activeOps[activeOps.length - 1]}`
    : activeOps[0] || 'file operation'

  return (
    <Dialog open onClose={() => blocker.reset()}>
      <DialogTitle>Operation in progress</DialogTitle>
      <DialogContent>
        <DialogContentText sx={{ fontSize: '0.875rem' }}>
          A file {opText} operation is in progress. If you leave this page, the operation will be cancelled.
        </DialogContentText>
        {isDeleting && (
          <DialogContentText sx={{ fontSize: '0.875rem', mt: 1 }}>
            If delete is interrupted, the file will be irrecoverable and orphaned data may remain in storage.
          </DialogContentText>
        )}
        <DialogContentText sx={{ fontSize: '0.875rem', mt: 1 }}>
          Do you want to leave anyway?
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
