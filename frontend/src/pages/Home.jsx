import React, { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAppStore } from '../store.jsx'
import StatusBadge from '../components/StatusBadge'
import { StartBackup, ChooseDestination, CancelCopy } from '../../wailsjs/go/services/CopyService'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

export default function Home() {
  const store = useAppStore()
  const navigate = useNavigate()
  const { prereqReport, failedCheckIds } = store
  
  const [isStarting, setIsStarting] = useState(false)
  const [lastError, setLastError] = useState(null)
  const [jobStatus, setJobStatus] = useState(null) // { id, state, message }
  const [destinationPath, setDestinationPath] = useState('')
  const [sourcePath, setSourcePath] = useState('')

  const canRunActions = prereqReport?.overallStatus !== 'fail'
  const isJobRunning = jobStatus?.state === 'running'

  // Listen for job status updates
  useEffect(() => {
    const cleanup = EventsOn('job:status', (data) => {
      console.log('[Home] Job status update:', data)
      setJobStatus(data)
      // Update paths if provided in status
      if (data.sourcePath) {
        setSourcePath(data.sourcePath)
      }
      if (data.destPath) {
        setDestinationPath(data.destPath)
      }
      if (data.state === 'completed' || data.state === 'failed' || data.state === 'cancelled') {
        setIsStarting(false)
      }
    })

    const errorCleanup = EventsOn('job:error', (data) => {
      console.error('[Home] Job error:', data)
      setLastError(data.message || 'An error occurred')
      setJobStatus(prev => prev ? { ...prev, state: 'failed', message: data.message } : null)
      setIsStarting(false)
    })

    return () => {
      cleanup()
      errorCleanup()
    }
  }, [])

  const handleChooseDestination = async () => {
    try {
      const path = await ChooseDestination()
      if (path) {
        setDestinationPath(path)
        return path
      }
      return null
    } catch (error) {
      console.error('[Home] Failed to choose destination:', error)
      setLastError(`Failed to choose destination: ${error.message || error}`)
      return null
    }
  }

  const handleStartBackup = async () => {
    console.log('[Home] handleStartBackup called')
    
    if (!canRunActions) {
      console.log('[Home] Prerequisites not met, navigating to prereqs')
      navigate('/prereqs')
      setTimeout(() => {
        if (failedCheckIds.length > 0) {
          const firstFailed = document.getElementById(`check-${failedCheckIds[0]}`)
          if (firstFailed) {
            firstFailed.scrollIntoView({ behavior: 'smooth', block: 'center' })
          }
        }
      }, 100)
      return
    }

    // Ensure destination is selected before starting
    let destPath = destinationPath
    if (!destPath) {
      destPath = await handleChooseDestination()
      if (!destPath) {
        // User cancelled directory selection
        return
      }
    }

    setIsStarting(true)
    setLastError(null)
    setJobStatus({ state: 'starting', message: 'Preparing backup...' })

    try {
      console.log('[Home] Calling StartBackup...')
      // Empty source = auto-detect, destPath is now set
      await StartBackup('', destPath, 'mount')
      console.log('[Home] Backup started successfully')
      // Don't set isStarting to false here - let the job:status event handle it
    } catch (error) {
      console.error('[Home] Failed to start backup:', error)
      setLastError(error.message || String(error))
      setJobStatus({ state: 'failed', message: error.message || String(error) })
      setIsStarting(false)
    }
  }

  const handleCancelBackup = async () => {
    try {
      await CancelCopy()
      setJobStatus(prev => prev ? { ...prev, state: 'cancelling', message: 'Cancelling backup...' } : null)
    } catch (error) {
      console.error('[Home] Failed to cancel backup:', error)
      setLastError(`Failed to cancel: ${error.message || error}`)
    }
  }

  const handleActionClick = (action) => {
    if (action === 'Copy') {
      handleStartBackup()
      return
    }
    
    if (!canRunActions) {
      navigate('/prereqs')
      return
    }
    
    // TODO: Implement other actions (Verify, Cleanup)
    alert(`${action} wizard NEVER COMING!`)
  }

  const getStatusColor = (state) => {
    switch (state) {
      case 'running': return '#2196f3'
      case 'completed': return '#4caf50'
      case 'failed': return '#f44336'
      case 'cancelled': return '#ff9800'
      case 'cancelling': return '#ff9800'
      default: return '#b0bec5'
    }
  }

  const getStatusIcon = (state) => {
    switch (state) {
      case 'running': return '⏳'
      case 'completed': return '✅'
      case 'failed': return '❌'
      case 'cancelled': return '⏹️'
      case 'cancelling': return '⏹️'
      default: return '📋'
    }
  }

  return (
    <div className="container">
      <h1>GusSync</h1>
      <p style={{ color: '#b0bec5', marginBottom: '24px' }}>
        Digs deep. Fetches everything. Never lets go.
      </p>

      {/* Overall Status Card */}
      <div className="card">
        <h2>Overall Status</h2>
        {prereqReport ? (
          <div style={{ marginTop: '16px' }}>
            <StatusBadge status={prereqReport.overallStatus} />
            <p style={{ marginTop: '12px', color: '#b0bec5' }}>
              {prereqReport.overallStatus === 'ok' && 'All prerequisites are met. Ready to backup!'}
              {prereqReport.overallStatus === 'warn' && 'Some prerequisites have warnings, but operations can proceed.'}
              {prereqReport.overallStatus === 'fail' && 'Some prerequisites are missing. Please fix them before proceeding.'}
            </p>
            {prereqReport.overallStatus === 'fail' && (
              <div style={{ marginTop: '12px', padding: '12px', background: '#f4433610', borderRadius: '4px', borderLeft: '4px solid #f44336' }}>
                <strong>Action Required:</strong> Please check the Prerequisites page to fix failed checks.
              </div>
            )}
          </div>
        ) : (
          <p style={{ color: '#b0bec5' }}>Loading prerequisites...</p>
        )}
      </div>

      {/* Backup Status Card */}
      {jobStatus && (
        <div className="card" style={{ borderLeft: `4px solid ${getStatusColor(jobStatus.state)}` }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div>
              <h2 style={{ margin: 0, display: 'flex', alignItems: 'center', gap: '8px' }}>
                {getStatusIcon(jobStatus.state)} Backup Status
              </h2>
              <p style={{ marginTop: '8px', color: '#b0bec5', marginBottom: 0 }}>
                {jobStatus.message || 'Processing...'}
              </p>
              {sourcePath && (
                <p style={{ marginTop: '4px', fontSize: '12px', color: '#90a4ae', marginBottom: 0 }}>
                  Source: <code style={{ fontSize: '11px' }}>{sourcePath}</code>
                </p>
              )}
              {destinationPath && (
                <p style={{ marginTop: '4px', fontSize: '12px', color: '#90a4ae', marginBottom: 0 }}>
                  Destination: <code style={{ fontSize: '11px' }}>{destinationPath}</code>
                </p>
              )}
            </div>
            {isJobRunning && (
              <button
                className="secondary"
                onClick={handleCancelBackup}
                style={{ minWidth: '100px' }}
              >
                Cancel
              </button>
            )}
          </div>
        </div>
      )}

      {/* Quick Actions */}
      <div className="card">
        <h2>Quick Actions</h2>
        <div style={{ display: 'flex', gap: '12px', marginTop: '16px', flexWrap: 'wrap' }}>
          <button
            className="primary"
            disabled={!canRunActions || isStarting || isJobRunning}
            onClick={handleStartBackup}
          >
            {isStarting ? 'Starting...' : isJobRunning ? 'Backup Running...' : 'Start Backup'}
          </button>
          {!destinationPath && (
            <button
              className="secondary"
              disabled={!canRunActions || isJobRunning}
              onClick={handleChooseDestination}
            >
              Choose Destination
            </button>
          )}
          {destinationPath && (
            <div style={{ 
              padding: '8px 12px', 
              background: '#1e3a5f', 
              borderRadius: '4px',
              fontSize: '12px',
              color: '#90caf9',
              display: 'flex',
              alignItems: 'center',
              gap: '8px'
            }}>
              <span>📁</span>
              <span style={{ maxWidth: '300px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {destinationPath}
              </span>
              <button
                onClick={handleChooseDestination}
                style={{
                  background: 'transparent',
                  border: 'none',
                  color: '#90caf9',
                  cursor: 'pointer',
                  padding: '2px 6px',
                  fontSize: '11px'
                }}
                title="Change destination"
              >
                ✏️
              </button>
            </div>
          )}
          <button
            className="primary"
            disabled={!canRunActions || isJobRunning}
            onClick={() => handleActionClick('Verify')}
          >
            Verify Backup
          </button>
          <button
            className="primary"
            disabled={!canRunActions || isJobRunning}
            onClick={() => handleActionClick('Cleanup')}
          >
            Cleanup Source
          </button>
        </div>
        {lastError && (
          <div style={{ marginTop: '12px', padding: '12px', background: '#f4433610', borderRadius: '4px', borderLeft: '4px solid #f44336' }}>
            <strong>Error:</strong> {lastError}
          </div>
        )}
        {!canRunActions && (
          <p style={{ marginTop: '12px', color: '#ff9800', fontSize: '14px' }}>
            ⚠️ Actions are disabled until prerequisites are met. Click a button to see what needs to be fixed.
          </p>
        )}
      </div>

      {/* Device Status (placeholder) */}
      <div className="card">
        <h2>Device Status</h2>
        <p style={{ color: '#b0bec5' }}>Device detection coming soon...</p>
      </div>
    </div>
  )
}
