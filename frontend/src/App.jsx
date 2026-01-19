import React, { useEffect } from 'react'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Sidebar from './components/Sidebar'
import Dashboard from './components/Dashboard'
import Prerequisites from './pages/Prerequisites'
import Logs from './pages/Logs'
import { AppStoreProvider, useAppStore } from './store.jsx'
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'
import { GetPrereqReport } from '../wailsjs/go/services/PrereqService'

function AppContent() {
  const store = useAppStore()
  
  // Log when component first mounts
  useEffect(() => {
    const mountTime = performance.now()
    const mountTimestamp = new Date().toISOString()
    console.log(`[TIMING ${mountTimestamp}] [App] ⭐ COMPONENT MOUNT ⭐ - AppContent component mounted at ${mountTime.toFixed(2)}ms`)
  }, [])

  useEffect(() => {
    // Robust initialization: wait for Wails runtime and bindings
    const initialize = async () => {
      console.log('[App] Starting initialization...')
      
      // Step 1: Wait for Wails runtime (window.runtime)
      let runtimeRetries = 0
      while (!window.runtime && runtimeRetries < 20) {
        await new Promise(r => setTimeout(r, 100))
        runtimeRetries++
      }

      if (!window.runtime) {
        console.error('[App] Wails runtime (window.runtime) never loaded.')
        // Fallback for browser mode if needed, but for Wails it's fatal
        return
      }

      console.log(`[App] Wails runtime ready after ${runtimeRetries * 100}ms`)

      // Step 2: Setup event listeners
      // Now safe to call EventsOn as window.runtime is guaranteed
      const cleanupLog = EventsOn('LogLine', (data) => {
        store.addLog(data)
      })

      const cleanupCheckProgress = EventsOn('PrereqCheckProgress', (data) => {
        const checkID = data?.checkID
        const status = data?.status
        
        if (!checkID) return
        
        if (status === 'starting') {
          store.setCurrentCheckID(checkID)
          store.setCheckProgress(checkID, 'starting')
        } else if (status === 'completed') {
          store.setCheckProgress(checkID, 'completed')
          setTimeout(() => {
            store.setCurrentCheckID((prev) => prev === checkID ? null : prev)
          }, 200)
        }
      })

      const cleanupPrereqReport = EventsOn('PrereqReport', (report) => {
        console.log('[App] Received PrereqReport event:', report)
        if (report && report.overallStatus) {
          store.setPrereqReport(report)
        }
      })

      // Step 3: Wait for service bindings and fetch initial data
      const initializePrereqReport = async () => {
        // Wait for window.go.services.PrereqService to be available
        let bindingRetries = 0
        while ((!window.go || !window.go.services || !window.go.services.PrereqService) && bindingRetries < 20) {
          await new Promise(r => setTimeout(r, 100))
          bindingRetries++
        }

        if (!window.go?.services?.PrereqService) {
          console.error('[App] PrereqService bindings never loaded. Path: window.go.services.PrereqService')
          return
        }

        console.log(`[App] PrereqService ready after ${bindingRetries * 100}ms`)

        let report = null
        let attempts = 0
        const maxPollAttempts = 15
        const pollInterval = 100

        try {
          console.log('[App] Requesting PrereqReport (initial query)...')
          // Use imported function which is a wrapper
          report = await GetPrereqReport()
          console.log('[App] Received Report (initial):', report)
        } catch (err) {
          console.error('[App] Initial fetch failed:', err)
        }

        while ((!report || !report.overallStatus) && attempts < maxPollAttempts) {
          await new Promise(r => setTimeout(r, pollInterval))
          attempts++
          
          try {
            report = await GetPrereqReport()
            if (report && report.overallStatus) {
              console.log(`[App] PrereqReport ready after ${attempts} poll attempts:`, report)
              break
            }
          } catch (err) {
            console.error(`[App] Poll attempt ${attempts} failed:`, err)
          }
        }

        if (report && report.overallStatus) {
          store.setPrereqReport(report)
        }
      }

      await initializePrereqReport()

      // Store cleanups so they can be called on unmount
      window._wails_cleanups = () => {
        cleanupLog()
        cleanupCheckProgress()
        cleanupPrereqReport()
      }
    }

    initialize()

    return () => {
      if (window._wails_cleanups) {
        window._wails_cleanups()
        delete window._wails_cleanups
      }
    }
  }, []) // Run once on mount
  
  // Reset check progress when prereqReport becomes null
  useEffect(() => {
    if (!store.prereqReport) {
      console.log('[App] Resetting check progress - prereqReport is null')
      // Reset all progress - clear each check
      const progressKeys = Object.keys(store.checkProgress || {})
      progressKeys.forEach(checkID => {
        store.setCheckProgress(checkID, 'pending')
      })
      store.setCurrentCheckID(null)
    }
  }, [store.prereqReport]) // Only depend on prereqReport, not entire store

  return (
    <div className="flex h-screen bg-slate-950 overflow-hidden">
      {/* Fixed Sidebar */}
      <Sidebar />

      {/* Main Content Area */}
      <main className="flex-1 ml-[250px] overflow-y-auto">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/prereqs" element={<Prerequisites />} />
          <Route path="/logs" element={<Logs />} />
        </Routes>
      </main>
    </div>
  )
}

function App() {
  return (
    <AppStoreProvider>
      <BrowserRouter>
        <AppContent />
      </BrowserRouter>
    </AppStoreProvider>
  )
}

export default App
