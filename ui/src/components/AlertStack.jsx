import { createContext, useCallback, useContext, useReducer } from 'react'
import { Alert, Stack, Slide } from '@mui/material'

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
          top: 64,
          right: 20,
          zIndex: 99999,
          width: 340,
          maxWidth: 'calc(100vw - 40px)',
        }}
      >
        {alerts.map((alert) => (
          <Slide key={alert.id} direction="left" in mountOnEnter unmountOnExit>
            <Alert
              severity={alert.severity}
              onClose={() => dispatch({ type: 'REMOVE', id: alert.id })}
              sx={{ fontSize: '0.8125rem' }}
            >
              {alert.message}
            </Alert>
          </Slide>
        ))}
      </Stack>
    </AlertContext.Provider>
  )
}

export function useAlert() {
  return useContext(AlertContext)
}
