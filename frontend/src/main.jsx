import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './index.css'

// DEBUGGING: Add visible status to body
function showStatus(msg, color = 'lime') {
  const status = document.createElement('div')
  status.style.cssText = `position: fixed; top: 10px; left: 10px; color: ${color}; background: rgba(0,0,0,0.9); padding: 10px; z-index: 99999; font-family: monospace; font-size: 12px; max-width: 90vw; white-space: pre-wrap;`
  status.textContent = msg
  document.body.appendChild(status)
}

// Global error handler
window.addEventListener('error', (e) => {
  showStatus(`GLOBAL ERROR: ${e.message}\n${e.filename}:${e.lineno}`, 'red')
})

window.addEventListener('unhandledrejection', (e) => {
  showStatus(`UNHANDLED REJECTION: ${e.reason}`, 'orange')
})

try {
  showStatus('Step 1: main.jsx is executing...', 'yellow')
  
  // DEBUGGING: Check if script is even loading
  console.log('[DEBUG] main.jsx is executing!')
  console.log('[DEBUG] window.runtime exists?', !!window.runtime)
  console.log('[DEBUG] window.go exists?', !!window.go)

  // Log when React starts rendering
  const reactStartTime = performance.now()
  const reactStartTimestamp = new Date().toISOString()
  console.log(`[TIMING ${reactStartTimestamp}] [React] ⭐ REACT START ⭐ - ReactDOM.createRoot called at ${reactStartTime.toFixed(2)}ms`)

  showStatus('Step 2: Looking for root element...', 'yellow')
  
  const rootElement = document.getElementById('root')
  console.log('[DEBUG] root element found?', !!rootElement)

  if (!rootElement) {
    console.error('[ERROR] Root element not found! Cannot render React app.')
    showStatus('ERROR: Root element not found!', 'red')
  } else {
    showStatus('Step 3: Root element found, creating React root...', 'yellow')
    
    const root = ReactDOM.createRoot(rootElement)

    const rootTime = performance.now()
    console.log(`[TIMING ${new Date().toISOString()}] [React] ReactDOM.createRoot completed (took ${(rootTime - reactStartTime).toFixed(2)}ms)`)

    showStatus('Step 4: Rendering App component...', 'yellow')
    
    // NOTE: React.StrictMode causes double renders in development, which can add delay
    // Disabled to reduce startup delay - enable only if needed for development testing
    console.log('[DEBUG] About to render App component...')
    root.render(<App />)
    console.log('[DEBUG] App component rendered successfully!')
    
    showStatus('Step 5: React render() called successfully!', 'lime')
    
    // Remove status after 2 seconds if all went well
    setTimeout(() => {
      const statuses = document.querySelectorAll('body > div[style*="position: fixed"]')
      statuses.forEach(s => s.remove())
    }, 2000)

    const renderTime = performance.now()
    console.log(`[TIMING ${new Date().toISOString()}] [React] root.render() called (took ${(renderTime - rootTime).toFixed(2)}ms from root creation)`)
  }
} catch (err) {
  console.error('[ERROR] Fatal error in main.jsx:', err)
  showStatus(`FATAL ERROR: ${err.message}\nStack: ${err.stack}`, 'red')
}


