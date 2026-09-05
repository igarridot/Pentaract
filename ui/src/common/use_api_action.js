import { useCallback } from 'react'
import { runWithAlert } from './api_action'

// useApiAction(addAlert) returns run(action, { success }) which awaits the
// call and reports failures through the alert stack.
export function useApiAction(addAlert) {
  return useCallback((action, options) => runWithAlert(addAlert, action, options), [addAlert])
}
