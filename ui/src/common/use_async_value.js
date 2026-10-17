import { useEffect, useRef, useState } from 'react'

// Loads a value once on mount. `load` must be a stable function (an API
// method, not an inline arrow) and `fallback` is what the value is while
// loading and whenever the load fails. Updates are dropped after unmount.
export function useAsyncValue(load, fallback) {
  const [value, setValue] = useState(fallback)
  const fallbackRef = useRef(fallback)

  useEffect(() => {
    let cancelled = false
    load().then(
      (data) => { if (!cancelled) setValue(data ?? fallbackRef.current) },
      () => { if (!cancelled) setValue(fallbackRef.current) },
    )
    return () => { cancelled = true }
  }, [load])

  return value
}
