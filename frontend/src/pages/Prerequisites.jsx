import React, { useEffect, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { useAppStore } from '../store.jsx'
import CheckCard from '../components/CheckCard'
import StatusBadge from '../components/StatusBadge'
import { RefreshNow } from '../../wailsjs/go/services/PrereqService'

export default function Prerequisites() {
  const store = useAppStore()
  const location = useLocation()
  const { prereqReport } = store
  const [refreshing, setRefreshing] = useState(false)
  const [toast, setToast] = useState(null)

  // Log on mount
  useEffect(() => {
    console.log('[Prerequisites] Component mounted')
    if (window.go?.services?.PrereqService) {
      console.log('[Prerequisites] PrereqService binding detected successfully')
    } else {
      console.warn('[Prerequisites] PrereqService binding NOT detected globally. This may be normal if using imports.')
    }
  }, [])

  // Show toast for a few seconds
  useEffect(() => {
    if (toast) {
      const timer = setTimeout(() => setToast(null), 3000)
      return () => clearTimeout(timer)
    }
  }, [toast])

  const handleRefresh = async () => {
    console.log('[Prerequisites] handleRefresh clicked')
    
    // Standard Wails pattern: Use imported functions from bindings
    // These functions handle the window.go... path internally
    setRefreshing(true)
    try {
      console.log('[Prerequisites] Calling RefreshNow binding...')
      const report = await RefreshNow()
      console.log('[Prerequisites] RefreshNow response:', report)
      
      if (report && report.overallStatus) {
        store.setPrereqReport(report)
        setToast('Prerequisites refreshed!')
      } else {
        console.warn('[Prerequisites] RefreshNow returned empty or invalid report:', report)
        setToast('Refresh completed, but status unknown')
      }
    } catch (err) {
      console.error('[Prerequisites] Failed to refresh prerequisites:', err)
      setToast(`Error: ${err.message || String(err)}`)
    } finally {
      setRefreshing(false)
    }
  }

  const failedChecks = prereqReport?.checks?.filter(c => c.status === 'fail') || []
  const warnChecks = prereqReport?.checks?.filter(c => c.status === 'warn') || []
  const okChecks = prereqReport?.checks?.filter(c => c.status === 'ok') || []

  return (
    <div className="min-h-screen bg-slate-950 text-white p-6">
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-3xl font-bold text-white">Prerequisites</h1>
        <button
          className="px-4 py-2 bg-slate-700 text-slate-200 rounded-lg hover:bg-slate-600 transition-colors disabled:opacity-50 disabled:cursor-not-allowed font-medium"
          onClick={handleRefresh}
          disabled={refreshing}
        >
          {refreshing ? 'Refreshing...' : 'Re-run Checks'}
        </button>
      </div>

      {prereqReport ? (
        <>
          {/* Overall Status */}
          <div className="bg-slate-800 rounded-lg p-6 mb-6">
            <h2 className="text-xl font-semibold text-white mb-4">Overall Status</h2>
            <div className="mt-4">
              <StatusBadge status={prereqReport.overallStatus} />
              <p className="mt-3 text-slate-300">
                {failedChecks.length > 0 && `${failedChecks.length} failed, `}
                {warnChecks.length > 0 && `${warnChecks.length} warnings, `}
                {okChecks.length} passed
              </p>
            </div>
          </div>

          {/* Failed Checks */}
          {failedChecks.length > 0 && (
            <div className="mb-6">
              <h2 className="text-xl font-semibold text-red-400 mb-4">
                Failed Checks ({failedChecks.length})
              </h2>
              {failedChecks.map((check) => (
                <CheckCard key={check.id} check={check} highlight={location.state?.highlight === check.id} />
              ))}
            </div>
          )}

          {/* Warning Checks */}
          {warnChecks.length > 0 && (
            <div className="mb-6">
              <h2 className="text-xl font-semibold text-amber-400 mb-4">
                Warnings ({warnChecks.length})
              </h2>
              {warnChecks.map((check) => (
                <CheckCard key={check.id} check={check} />
              ))}
            </div>
          )}

          {/* Passed Checks */}
          {okChecks.length > 0 && (
            <div className="mb-6">
              <h2 className="text-xl font-semibold text-emerald-400 mb-4">
                Passed Checks ({okChecks.length})
              </h2>
              {okChecks.map((check) => (
                <CheckCard key={check.id} check={check} />
              ))}
            </div>
          )}
        </>
      ) : (
        <div className="bg-slate-800 rounded-lg p-6">
          <p className="text-slate-300">Loading prerequisites...</p>
        </div>
      )}

      {/* Toast */}
      {toast && (
        <div className="fixed bottom-6 right-6 bg-slate-800 text-white px-4 py-3 rounded-lg shadow-lg border border-slate-700 z-50">
          {toast}
        </div>
      )}
    </div>
  )
}

