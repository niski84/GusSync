import React, { createContext, useContext, useState, useCallback, useMemo, useRef } from 'react'

const AppStoreContext = createContext(null)

export function AppStoreProvider({ children }) {
  const [prereqReport, setPrereqReport] = useState(null)
  const [logs, setLogs] = useState([])
  const [checkProgress, setCheckProgress] = useState({}) // Track check progress: { checkID: 'starting' | 'completed' }
  const [currentCheckID, setCurrentCheckID] = useState(null) // Current check being run
  const maxLogs = 2000

  // Track last seen seq for out-of-order protection
  const lastPrereqSeqRef = useRef(0)

  const addLog = useCallback((logEntry) => {
    setLogs((prev) => {
      const newLogs = [...prev, logEntry]
      // Keep only last maxLogs entries
      if (newLogs.length > maxLogs) {
        return newLogs.slice(-maxLogs)
      }
      return newLogs
    })
  }, [])

  const updateCheckProgress = useCallback((checkID, status) => {
    if (!checkID) {
      console.warn('[Store] updateCheckProgress called without checkID')
      return
    }
    console.log('[Store] updateCheckProgress:', checkID, status)
    setCheckProgress((prev) => {
      const updated = { ...prev, [checkID]: status }
      console.log('[Store] Updated checkProgress state:', updated)
      return updated
    })
  }, [])

  const updateCurrentCheckID = useCallback((checkID) => {
    console.log('[Store] updateCurrentCheckID:', checkID)
    setCurrentCheckID(checkID || null)
  }, [])

  // Wrapper for setPrereqReport that enforces seq-based ordering
  const updatePrereqReport = useCallback((report) => {
    if (!report) {
      setPrereqReport(null)
      lastPrereqSeqRef.current = 0
      return
    }
    
    const reportSeq = report.seq || 0
    if (reportSeq > 0 && reportSeq <= lastPrereqSeqRef.current) {
      console.log('[Store] Ignoring out-of-order prereq report, seq:', reportSeq, 'lastSeen:', lastPrereqSeqRef.current)
      return
    }
    lastPrereqSeqRef.current = reportSeq
    setPrereqReport(report)
  }, [])

  // Memoize the store object to maintain stable reference across renders
  // This prevents useEffect dependencies from triggering re-subscriptions
  const store = useMemo(() => ({
    prereqReport,
    setPrereqReport: updatePrereqReport, // Use seq-protected wrapper
    logs,
    addLog,
    checkProgress,
    setCheckProgress: updateCheckProgress, // Use wrapper function
    currentCheckID,
    setCurrentCheckID: updateCurrentCheckID, // Use wrapper function
    failedCheckIds: prereqReport?.checks?.filter(c => c.status === 'fail').map(c => c.id) || [],
  }), [
    prereqReport,
    logs,
    checkProgress,
    currentCheckID,
    addLog,
    updatePrereqReport,
    updateCheckProgress,
    updateCurrentCheckID,
  ])

  return (
    <AppStoreContext.Provider value={store}>
      {children}
    </AppStoreContext.Provider>
  )
}

export function useAppStore() {
  const context = useContext(AppStoreContext)
  if (!context) {
    throw new Error('useAppStore must be used within AppStoreProvider')
  }
  return context
}

