// Runs an API call and turns its outcome into alerts: the error message on
// failure, an optional success message otherwise. onSuccess receives the
// result; onError may return true to signal it handled the failure itself.
// Returns the result, or undefined when the call failed.
export async function runWithAlert(addAlert, action, { success, onSuccess, onError } = {}) {
  try {
    const result = await action()
    if (success) addAlert(success, 'success')
    if (onSuccess) onSuccess(result)
    return result
  } catch (err) {
    if (onError && onError(err) === true) return undefined
    addAlert(err.message, 'error')
    return undefined
  }
}
