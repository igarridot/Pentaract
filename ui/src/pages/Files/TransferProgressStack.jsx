import UploadProgress from '../../components/UploadProgress'
import DownloadProgress from '../../components/DownloadProgress'
import DeleteProgress from '../../components/DeleteProgress'
import BulkOperationProgress from '../../components/BulkOperationProgress'

// Renders all in-flight transfer progress widgets (uploads, downloads, the
// active delete, and the bulk operation). Pure presentation — state and
// cancel callbacks are owned by the hooks in Files/index.jsx. The state
// objects use the progress components' prop names, so they spread straight in.
export default function TransferProgressStack({
  uploadStates,
  onCancelUpload,
  downloadStates,
  onCancelDownload,
  deleteState,
  bulkOperation,
  bulkMetrics,
  onCancelBulk,
}) {
  return (
    <>
      {uploadStates.map((uploadState) => (
        <UploadProgress key={uploadState.id} {...uploadState} onCancel={() => onCancelUpload(uploadState.id)} />
      ))}
      {downloadStates.map((downloadState) => (
        <DownloadProgress key={downloadState.id} {...downloadState} onCancel={() => onCancelDownload(downloadState.id)} />
      ))}
      {deleteState && <DeleteProgress {...deleteState} />}
      {bulkOperation && (
        <BulkOperationProgress
          {...bulkOperation}
          {...bulkMetrics}
          onCancel={bulkOperation.status === 'running' ? onCancelBulk : null}
        />
      )}
    </>
  )
}
