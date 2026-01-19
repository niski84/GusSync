import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './index.css'

// Log when React starts rendering
const reactStartTime = performance.now()
const reactStartTimestamp = new Date().toISOString()
console.log(`[TIMING ${reactStartTimestamp}] [React] ⭐ REACT START ⭐ - ReactDOM.createRoot called at ${reactStartTime.toFixed(2)}ms`)

const root = ReactDOM.createRoot(document.getElementById('root'))

const rootTime = performance.now()
console.log(`[TIMING ${new Date().toISOString()}] [React] ReactDOM.createRoot completed (took ${(rootTime - reactStartTime).toFixed(2)}ms)`)

// NOTE: React.StrictMode causes double renders in development, which can add delay
// Disabled to reduce startup delay - enable only if needed for development testing
root.render(<App />)

const renderTime = performance.now()
console.log(`[TIMING ${new Date().toISOString()}] [React] root.render() called (took ${(renderTime - rootTime).toFixed(2)}ms from root creation)`)


