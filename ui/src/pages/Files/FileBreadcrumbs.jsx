import PathBreadcrumbs from '../../components/PathBreadcrumbs'

// Path navigation for the file browser. onNavigate receives the absolute
// route to navigate to.
export default function FileBreadcrumbs({ prefix, pathParts, onNavigate }) {
  return (
    <PathBreadcrumbs
      parts={pathParts}
      sx={{ mb: 2 }}
      onSelect={(parts) => onNavigate(parts.length ? `${prefix}${parts.join('/')}/` : prefix)}
    />
  )
}
