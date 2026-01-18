import React, { useEffect } from 'react'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Sidebar from './components/Sidebar'
import Dashboard from './components/Dashboard'
import Prerequisites from './pages/Prerequisites'
import Logs from './pages/Logs'
import { AppStoreProvider, useAppStore } from './store.jsx'
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'

function AppContent() {
  const store = useAppStore()

  useEffect(() => {
    // Subscribe to Wails events
    if (window.runtime) {
      // PrereqReport event
      const cleanupPrereq = EventsOn('PrereqReport', (data) => {
        console.log('[App] PrereqReport event:', data)
        store.setPrereqReport(data)
      })

      // PrereqCheckProgress event - track individual check progress
      const cleanupCheckProgress = EventsOn('PrereqCheckProgress', (data) => {
        console.log('[App] PrereqCheckProgress event received:', JSON.stringify(data))
        
        const checkID = data?.checkID
        const status = data?.status
        
        if (!checkID) {
          console.warn('[App] PrereqCheckProgress event missing checkID:', data)
          return
        }
        
        if (status === 'starting') {
          console.log('[App] Check starting:', checkID)
          store.setCurrentCheckID(checkID)
          store.setCheckProgress(checkID, 'starting')
        } else if (status === 'completed') {
          console.log('[App] Check completed:', checkID)
          store.setCheckProgress(checkID, 'completed')
          // Clear current check after a brief delay to show completion
          setTimeout(() => {
            if (store.currentCheckID === checkID) {
              store.setCurrentCheckID(null)
            }
          }, 200)
        } else {
          console.warn('[App] Unknown PrereqCheckProgress status:', status)
        }
      })

      // LogLine event
      const cleanupLog = EventsOn('LogLine', (data) => {
        console.log('[App] LogLine event:', data)
        store.addLog(data)
      })

      // Cleanup on unmount
      return () => {
        cleanupPrereq()
        cleanupCheckProgress()
        cleanupLog()
      }
    } else {
      console.warn('[App] Wails runtime not available - running in browser mode')
    }
  }, [store])

  useEffect(() => {
    // Load initial prereq report on mount
    // Add small delay to ensure event listeners are set up first
    const timer = setTimeout(() => {
      if (window.PrereqService) {
        window.PrereqService.GetPrereqReport()
          .then((report) => {
            console.log('[App] Initial PrereqReport:', report)
            store.setPrereqReport(report)
          })
          .catch((err) => {
            console.error('[App] Failed to get initial PrereqReport:', err)
          })
      }
    }, 100) // Small delay to ensure listeners are ready
    
    return () => clearTimeout(timer)
  }, [store])
  
  // Reset check progress when prereqReport becomes null
  useEffect(() => {
    if (!store.prereqReport) {
      console.log('[App] Resetting check progress - prereqReport is null')
      // Reset all progress - clear each check
      Object.keys(store.checkProgress || {}).forEach(checkID => {
        store.setCheckProgress(checkID, 'pending')
      })
      store.setCurrentCheckID(null)
    }
  }, [store, store.prereqReport])

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
