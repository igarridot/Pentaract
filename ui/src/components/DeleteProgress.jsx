import { Typography } from '@mui/material'
import { calculatePercent } from '../common/progress'
import ProgressCard from './ProgressCard'

export default function DeleteProgress({ label, totalChunks, deletedChunks, status, workersStatus }) {
  const isActive = status === 'deleting'
  const isError = status === 'error'
  const percent = calculatePercent(deletedChunks, totalChunks)
  const pending = totalChunks > 0 ? Math.max(totalChunks - deletedChunks, 0) : 0
  const workersText = workersStatus === 'waiting_rate_limit' ? 'Workers waiting (rate limit)' : 'Workers active'

  const title = isError ? 'Delete failed' : isActive ? 'Deleting' : 'Delete complete'

  return (
    <ProgressCard
      title={title}
      subtitle={label}
      percent={percent}
      variant={totalChunks > 0 ? 'determinate' : 'indeterminate'}
      progressColor={isError ? 'error' : isActive ? 'primary' : 'success'}
      isError={isError}
    >
      <Typography variant="caption" color="text.secondary">
        {totalChunks > 0 ? `${workersText} · ${deletedChunks}/${totalChunks} chunks · ${pending} pending` : `${workersText} · Calculating chunks...`}
      </Typography>
    </ProgressCard>
  )
}
