import UploadProgress from '../../components/UploadProgress'
import DownloadProgress from '../../components/DownloadProgress'
import DeleteProgress from '../../components/DeleteProgress'
import BulkOperationProgress from '../../components/BulkOperationProgress'

// Renders all in-flight transfer progress widgets (uploads, downloads, the
// active delete, and the bulk operation). Pure presentation — state and
// cancel callbacks are owned by the hooks in Files/index.jsx.
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
        <UploadProgress
          key={uploadState.id}
          filename={uploadState.filename}
          totalBytes={uploadState.totalBytes}
          uploadedBytes={uploadState.uploadedBytes}
          totalChunks={uploadState.totalChunks}
          uploadedChunks={uploadState.uploadedChunks}
          verificationTotal={uploadState.verificationTotal}
          verifiedChunks={uploadState.verifiedChunks}
          status={uploadState.status}
          workersStatus={uploadState.workersStatus}
          onCancel={() => onCancelUpload(uploadState.id)}
        />
      ))}
      {downloadStates.map((downloadState) => (
        <DownloadProgress
          key={downloadState.id}
          filename={downloadState.filename}
          totalBytes={downloadState.totalBytes}
          downloadedBytes={downloadState.downloadedBytes}
          totalChunks={downloadState.totalChunks}
          downloadedChunks={downloadState.downloadedChunks}
          status={downloadState.status}
          workersStatus={downloadState.workersStatus}
          errorMessage={downloadState.errorMessage}
          onCancel={() => onCancelDownload(downloadState.id)}
        />
      ))}
      {deleteState && (
        <DeleteProgress
          label={deleteState.label}
          totalChunks={deleteState.totalChunks}
          deletedChunks={deleteState.deletedChunks}
          status={deleteState.status}
          workersStatus={deleteState.workersStatus}
        />
      )}
      {bulkOperation && (
        <BulkOperationProgress
          operation={bulkOperation.operation}
          status={bulkOperation.status}
          total={bulkOperation.total}
          completed={bulkOperation.completed}
          totalBytes={bulkMetrics.totalBytes}
          processedBytes={bulkMetrics.processedBytes}
          totalChunks={bulkMetrics.totalChunks}
          processedChunks={bulkMetrics.processedChunks}
          workersStatus={bulkMetrics.workersStatus}
          onCancel={bulkOperation.status === 'running' ? onCancelBulk : null}
        />
      )}
    </>
  )
}
