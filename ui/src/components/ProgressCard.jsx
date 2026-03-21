import { Box, LinearProgress, Typography, IconButton, Button } from '@mui/material'
import { Close as CloseIcon } from '@mui/icons-material'

/**
 * Shared progress card layout used by Upload, Download, Delete, and BulkOperation progress components.
 *
 * @param {string}  title              - Status text (e.g. "Uploading", "Download failed")
 * @param {string}  [subtitle]         - Secondary text shown after the title (e.g. filename)
 * @param {number}  percent            - 0-100 progress value
 * @param {'determinate'|'indeterminate'} [variant='determinate']
 * @param {string}  [progressColor='primary'] - MUI color for the LinearProgress bar
 * @param {boolean} [isError=false]
 * @param {boolean} [isWarning=false]  - Cancelled / warning styling
 * @param {React.ReactNode} children   - Caption row content
 * @param {React.ReactNode} [afterBar] - Extra content rendered after the caption row (e.g. error message)
 * @param {Function} [onCancel]        - If provided together with cancelLabel, renders a cancel control
 * @param {string}  [cancelLabel]      - Text for a Button-style cancel; omit for icon-style cancel
 * @param {boolean} [showCancel=false] - Whether to actually show the cancel control
 * @param {object}  [titleSx]         - Extra sx overrides for the title Typography
 * @param {object}  [containerSx]     - Extra sx merged into the outer Box
 */
export default function ProgressCard({
  title,
  subtitle,
  percent,
  variant = 'determinate',
  progressColor = 'primary',
  isError = false,
  isWarning = false,
  children,
  afterBar,
  onCancel,
  cancelLabel,
  showCancel = false,
  titleSx,
  containerSx,
}) {
  const bgColor = isError
    ? 'rgba(255,59,48,0.06)'
    : isWarning
      ? 'rgba(255,152,0,0.08)'
      : 'background.paper'

  const borderColor = isError
    ? 'rgba(255,59,48,0.15)'
    : isWarning
      ? 'rgba(255,152,0,0.2)'
      : 'divider'

  return (
    <Box
      sx={{
        width: '100%',
        maxWidth: '100%',
        boxSizing: 'border-box',
        mb: 2,
        p: 2,
        bgcolor: bgColor,
        borderRadius: 3,
        border: '1px solid',
        borderColor,
        overflow: 'hidden',
        ...containerSx,
      }}
    >
      {/* Title row */}
      <Box sx={{ display: 'flex', alignItems: { xs: 'flex-start', sm: 'center' }, justifyContent: 'space-between', mb: 1, minWidth: 0 }}>
        <Typography variant="body2" sx={{ fontWeight: 500, flexGrow: 1, minWidth: 0, pr: showCancel && !cancelLabel ? 1 : 0, wordBreak: 'break-word', ...titleSx }}>
          {title}
          {subtitle != null && (
            <Typography
              component="span"
              variant="body2"
              color="text.secondary"
              sx={{ ml: 0.5, maxWidth: '100%', overflowWrap: 'anywhere' }}
            >
              {subtitle}
            </Typography>
          )}
        </Typography>
        {showCancel && onCancel && !cancelLabel && (
          <IconButton size="small" onClick={onCancel} sx={{ ml: 1, opacity: 0.5, '&:hover': { opacity: 1 } }}>
            <CloseIcon sx={{ fontSize: 16 }} />
          </IconButton>
        )}
      </Box>

      {/* Progress bar */}
      <LinearProgress
        variant={variant}
        value={percent}
        color={progressColor}
        sx={{ mb: 0.75, width: '100%' }}
      />

      {/* Caption / detail row */}
      {children}

      {/* Optional extra content (error messages, etc.) */}
      {afterBar}

      {/* Button-style cancel */}
      {showCancel && onCancel && cancelLabel && (
        <Button size="small" color="warning" onClick={onCancel} sx={{ mt: 1 }}>
          {cancelLabel}
        </Button>
      )}
    </Box>
  )
}
