import { Breadcrumbs, Link as MuiLink } from '@mui/material'

const linkSx = { cursor: 'pointer', fontSize: '0.8125rem' }

// Root > a > b navigation. onSelect receives the selected path segments
// (empty array for Root) so each caller builds its own target path.
export default function PathBreadcrumbs({ parts, onSelect, sx }) {
  return (
    <Breadcrumbs sx={sx}>
      <MuiLink underline="hover" color="inherit" sx={linkSx} onClick={() => onSelect([])}>
        Root
      </MuiLink>
      {parts.map((part, i) => (
        <MuiLink
          key={parts.slice(0, i + 1).join('/')}
          underline="hover"
          color="inherit"
          sx={linkSx}
          onClick={() => onSelect(parts.slice(0, i + 1))}
        >
          {part}
        </MuiLink>
      ))}
    </Breadcrumbs>
  )
}
