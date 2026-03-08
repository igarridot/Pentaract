import { useEffect, useRef } from 'react'

export function useTransferSpeed(bytesTransferred) {
  const ref = useRef({ lastBytes: 0, lastTime: Date.now(), speed: 0 })

  useEffect(() => {
    const now = Date.now()
    const s = ref.current
    const elapsed = (now - s.lastTime) / 1000
    if (elapsed > 0.3 && bytesTransferred > s.lastBytes) {
      s.speed = (bytesTransferred - s.lastBytes) / elapsed
      s.lastBytes = bytesTransferred
      s.lastTime = now
    }
  }, [bytesTransferred])

  return ref.current.speed
}
