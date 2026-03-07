import { createContext, useCallback, useContext, useReducer } from 'react'
import { Alert, Stack } from '@mui/material'

const AlertContext = createContext()

function alertReducer(state, action) {
  switch (action.type) {
    case 'ADD':
      return [action.payload, ...state]
    case 'REMOVE':
      return state.filter((a) => a.id !== action.id)
    default:
      return state
  }
}

export function AlertProvider({ children }) {
  const [alerts, dispatch] = useReducer(alertReducer, [])

  const addAlert = useCallback((message, severity = 'info', { persistent = false } = {}) => {
    const id = Date.now() + Math.random()
    dispatch({ type: 'ADD', payload: { id, message, severity } })
    if (!persistent) {
      setTimeout(() => dispatch({ type: 'REMOVE', id }), 5000)
    }
  }, [])

  return (
    <AlertContext.Provider value={addAlert}>
      {children}
      <Stack
        spacing={1}
        sx={{
          position: 'fixed',
          top: 16,
          right: 16,
          zIndex: 99999,
          width: '30vw',
          minWidth: 240,
          maxWidth: 360,
        }}
      >
        {alerts.map((alert) => (
          <Alert
            key={alert.id}
            severity={alert.severity}
            onClose={() => dispatch({ type: 'REMOVE', id: alert.id })}
          >
            {alert.message}
          </Alert>
        ))}
      </Stack>
    </AlertContext.Provider>
  )
}

export function useAlert() {
  return useContext(AlertContext)
}
