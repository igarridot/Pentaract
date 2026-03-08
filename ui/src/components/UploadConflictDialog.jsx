import {
  Dialog, DialogTitle, DialogContent, DialogActions, Button, Typography, FormControlLabel, Checkbox,
} from '@mui/material'

export default function UploadConflictDialog({
  open,
  filename,
  targetPath,
  applyForAll,
  onApplyForAllChange,
  onDecision,
}) {
  const locationLabel = targetPath ? `/${targetPath}` : '/ (root)'

  return (
    <Dialog open={open} onClose={() => onDecision('skip', applyForAll)} maxWidth="xs" fullWidth>
      <DialogTitle>File already exists</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary">
          {`"${filename}" already exists in "${locationLabel}".`}
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
          Choose whether to skip this file or keep both by creating a renamed copy.
        </Typography>
        <FormControlLabel
          sx={{ mt: 1 }}
          control={(
            <Checkbox
              checked={applyForAll}
              onChange={(e) => onApplyForAllChange(e.target.checked)}
            />
          )}
          label="Apply for all"
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={() => onDecision('skip', applyForAll)} color="inherit">Skip</Button>
        <Button onClick={() => onDecision('keep_both', applyForAll)} variant="contained">Keep both</Button>
      </DialogActions>
    </Dialog>
  )
}
