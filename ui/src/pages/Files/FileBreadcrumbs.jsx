import { Breadcrumbs, Link as MuiLink } from '@mui/material'

const linkSx = { cursor: 'pointer', fontSize: '0.8125rem' }

// Renders the Root → … path navigation. onNavigate receives the absolute path
// to navigate to.
export default function FileBreadcrumbs({ prefix, pathParts, onNavigate }) {
  return (
    <Breadcrumbs sx={{ mb: 2 }}>
      <MuiLink
        key="root"
        underline="hover"
        color="inherit"
        sx={linkSx}
        onClick={() => onNavigate(prefix)}
      >
        Root
      </MuiLink>
      {pathParts.map((part, i) => {
        const pathTo = prefix + pathParts.slice(0, i + 1).join('/') + '/'
        return (
          <MuiLink
            key={pathTo}
            underline="hover"
            color="inherit"
            sx={linkSx}
            onClick={() => onNavigate(pathTo)}
          >
            {part}
          </MuiLink>
        )
      })}
    </Breadcrumbs>
  )
}
