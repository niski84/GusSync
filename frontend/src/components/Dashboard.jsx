import React, { useState } from 'react'
import { CheckCircle2, XCircle, Loader2 } from 'lucide-react'
import { useBackupState } from '../hooks/useBackupState'
import { StartBackup, ChooseDestination, CancelCopy } from '../../wailsjs/go/services/CopyService'
import { StartVerify } from '../../wailsjs/go/services/VerifyService'
import { StartCleanup } from '../../wailsjs/go/services/CleanupService'
import { useAppStore } from '../store.jsx'

export default function Dashboard() {
  const backupState = useBackupState()
  const store = useAppStore()
  const { prereqReport, checkProgress, currentCheckID } = store
  
  const [isStarting, setIsStarting] = useState(false)
  const [destinationPath, setDestinationPath] = useState('')
  const [error, setError] = useState(null)
  const [isVerifying, setIsVerifying] = useState(false)
  const [isCleaning, setIsCleaning] = useState(false)

  // Reset starting/loading states when backup state changes
  React.useEffect(() => {
    if (backupState.isRunning) {
      setIsStarting(false)
    }
    if (!backupState.isRunning && backupState.status !== 'idle') {
      setIsStarting(false)
      setIsVerifying(false)
      setIsCleaning(false)
    }
  }, [backupState.isRunning, backupState.status])
  
  // Default list of checks (in order)
  const defaultChecks = [
    { id: 'adb', name: 'Android Debug Bridge (ADB)' },
    { id: 'mtp_tools', name: 'MTP/GVFS Support' },
    { id: 'device_connection', name: 'Device Connection' },
    { id: 'destination_write', name: 'Destination Write Access' },
    { id: 'disk_space', name: 'Disk Space' },
    { id: 'webview2', name: 'WebView2 Runtime' },
    { id: 'filesystem_support', name: 'File System Support' },
  ]

  // Determine system readiness based on prerequisite report
  const canRunActions = prereqReport?.overallStatus !== 'fail'
  const systemReady = prereqReport?.overallStatus === 'ok'
  const isChecking = !prereqReport || (prereqReport && !prereqReport.overallStatus)

  const handleChooseDestination = async () => {
    try {
      const path = await ChooseDestination()
      if (path) {
        setDestinationPath(path)
        return path
      }
      return null
    } catch (error) {
      console.error('[Dashboard] Failed to choose destination:', error)
      setError(`Failed to choose destination: ${error.message || error}`)
      return null
    }
  }

  const handleStartBackup = async () => {
    console.log('[Dashboard] handleStartBackup called')
    console.log('[Dashboard] canRunActions:', canRunActions)
    
    if (!canRunActions) {
      console.log('[Dashboard] Cannot run actions, returning')
      return
    }

    // Ensure destination is selected
    let destPath = destinationPath || backupState.destPath
    console.log('[Dashboard] destPath:', destPath)
    
    if (!destPath) {
      console.log('[Dashboard] No destPath, opening dialog...')
      destPath = await handleChooseDestination()
      if (!destPath) {
        console.log('[Dashboard] User cancelled destination selection')
        return // User cancelled
      }
    }

    console.log('[Dashboard] About to call StartBackup with destPath:', destPath)
    setIsStarting(true)
    setError(null)

    try {
      console.log('[Dashboard] Calling StartBackup...')
      const taskId = await StartBackup('', destPath, 'mount')
      console.log('[Dashboard] Backup task started, taskId:', taskId)
      // Reset isStarting - if backend sent task:update, isRunning will be true
      // and UI will show "Running..." instead of "Start Backup"
      setIsStarting(false)
    } catch (error) {
      console.error('[Dashboard] Failed to start backup:', error)
      setError(error.message || String(error))
      setIsStarting(false)
    }
  }

  const handleCancelBackup = async () => {
    try {
      await CancelCopy()
    } catch (error) {
      console.error('[Dashboard] Failed to cancel backup:', error)
      setError(`Failed to cancel: ${error.message || error}`)
    }
  }

  const handleVerifyBackup = async () => {
    console.log('[Dashboard] handleVerifyBackup called')
    const sourcePath = backupState.sourcePath
    let destPath = destinationPath || backupState.destPath

    if (!destPath) {
      console.log('[Dashboard] No destPath, opening dialog...')
      destPath = await handleChooseDestination()
      if (!destPath) {
        console.log('[Dashboard] User cancelled destination selection')
        return
      }
    }

    setIsVerifying(true)
    setError(null)

    try {
      console.log('[Dashboard] Calling StartVerify...')
      // Level is currently ignored by backend but kept for future use
      const taskId = await StartVerify({
        sourcePath: sourcePath || '',
        destPath: destPath,
        mode: 'auto', // Backend will auto-detect available state files
      })
      console.log('[Dashboard] Verify task started, taskId:', taskId)
      setIsVerifying(false)
    } catch (error) {
      console.error('[Dashboard] Failed to start verification:', error)
      setError(`Failed to start verification: ${error.message || error}`)
      setIsVerifying(false)
    }
  }

  const handleCleanupSource = async () => {
    console.log('[Dashboard] handleCleanupSource called')
    const sourcePath = backupState.sourcePath
    let destPath = destinationPath || backupState.destPath

    if (!destPath) {
      console.log('[Dashboard] No destPath, opening dialog...')
      destPath = await handleChooseDestination()
      if (!destPath) {
        console.log('[Dashboard] User cancelled destination selection')
        return
      }
    }

    setIsCleaning(true)
    setError(null)

    try {
      console.log('[Dashboard] Calling StartCleanup...')
      const taskId = await StartCleanup({
        sourceRoot: sourcePath || '',
        destRoot: destPath,
        stateFiles: [], // Empty array means auto-detect
        processBoth: true, // Process both mount and adb state files if available
      })
      console.log('[Dashboard] Cleanup task started, taskId:', taskId)
      setIsCleaning(false)
    } catch (error) {
      console.error('[Dashboard] Failed to start cleanup:', error)
      setError(`Failed to start cleanup: ${error.message || error}`)
      setIsCleaning(false)
    }
  }

  // Get device info from status or prereq report
  const getDeviceInfo = () => {
    if (backupState.devices && backupState.devices.length > 0) {
      const device = backupState.devices[0]
      if (device.type === 'adb') {
        return { name: `${device.id}`, type: 'ADB' }
      }
      // For MTP, try to extract name from ID which is like mtp:host=Xiaomi...
      const match = device.id.match(/host=([^/]+)/)
      if (match) return { name: match[1], type: 'MTP' }
      return { name: device.name || device.id, type: device.type?.toUpperCase() || 'MTP' }
    }
    
    if (backupState.sourcePath) {
      // Extract device name from path like /run/user/1000/gvfs/mtp:host=Xiaomi_Mi_11_Ultra
      const match = backupState.sourcePath.match(/mtp:host=([^/]+)|gphoto2:host=([^/]+)/)
      if (match) {
        return { name: match[1] || match[2] || 'Unknown Device', type: 'MTP' }
      }
      return { name: 'Device Connected', type: 'MTP' }
    }

    // Fallback to PrereqReport details if available
    const deviceCheck = prereqReport?.checks?.find(c => c.id === 'device_connection' && c.status === 'ok')
    if (deviceCheck && deviceCheck.details) {
      if (deviceCheck.details.includes('ADB device connected')) {
        const parts = deviceCheck.details.split(': ')
        return { name: parts[1] || 'ADB Device', type: 'ADB' }
      }
      if (deviceCheck.details.includes('MTP/gphoto2 device mounted')) {
        const parts = deviceCheck.details.split(': ')
        return { name: parts[1] || 'MTP Device', type: 'MTP' }
      }
    }

    return { name: 'No Device', type: '' }
  }

  const deviceInfo = getDeviceInfo()

  // Truncate middle of path
  const truncatePath = (path, maxLength = 40) => {
    if (!path) return ''
    if (path.length <= maxLength) return path
    const half = Math.floor(maxLength / 2)
    return `${path.slice(0, half)}...${path.slice(-half)}`
  }

  return (
    <div className="min-h-screen bg-slate-950">
      {/* Main Header */}
      <header className="p-6 border-b border-slate-800">
        <h1 className="text-3xl font-bold text-white">GusSync Desktop</h1>
        <p className="text-slate-400 mt-2">Digs deep. Fetches everything. Never lets go.</p>
      </header>

      {/* Grid Layout */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6 p-6">
      {/* Card A: Status */}
      <div className="bg-slate-800 rounded-lg p-8 flex flex-col min-h-[300px]">
        {isChecking ? (
          <div className="flex flex-col items-center justify-center h-full">
            {/* Checking state - Knight Rider scanning effect */}
            <div className="w-full mb-4 relative h-16 bg-slate-900 rounded-lg overflow-hidden">
              {/* Scanning bar - Knight Rider effect */}
              <div className="absolute inset-0 knight-rider-scanner"></div>
            </div>
            <p className="text-2xl font-bold text-blue-400 mb-2">Checking...</p>
            <div className="text-slate-300 text-sm space-y-1 w-full max-w-xs">
              <p className="font-medium mb-2">Running system checks:</p>
              <ul className="text-xs space-y-1 ml-4">
                {defaultChecks.map((check) => {
                  const status = checkProgress[check.id] || 'pending'
                  const isCurrent = currentCheckID === check.id
                  const isCompleted = status === 'completed'
                  
                  let className = 'text-slate-400'
                  let icon = '• '
                  
                  if (isCompleted) {
                    className = 'text-emerald-400'
                    icon = '✓ '
                  } else if (isCurrent) {
                    className = 'text-blue-400 font-bold'
                    icon = '→ '
                  }
                  
                  return (
                    <li key={check.id} className={className}>
                      {icon}{check.name}
                    </li>
                  )
                })}
              </ul>
            </div>
          </div>
        ) : systemReady ? (
          <div className="flex flex-col items-center justify-center h-full">
            <CheckCircle2 size={64} className="text-emerald-500 mb-4" />
            <p className="text-2xl font-bold text-emerald-500 mb-2">OK</p>
            <p className="text-slate-400">System Ready</p>
          </div>
        ) : (
          <div className="w-full h-full flex flex-col">
            <div className="flex items-center gap-3 mb-6">
              {prereqReport?.overallStatus === 'fail' ? (
                <XCircle size={32} className="text-red-500" />
              ) : (
                <Loader2 size={32} className="text-amber-500 animate-spin" />
              )}
              <div>
                <h3 className={`text-xl font-bold ${prereqReport?.overallStatus === 'fail' ? 'text-red-500' : 'text-amber-500'}`}>
                  System {prereqReport?.overallStatus === 'fail' ? 'Check Failed' : 'Warning'}
                </h3>
                <p className="text-slate-400 text-sm">Please address the issues below</p>
              </div>
            </div>

            <div className="space-y-4 overflow-y-auto pr-2 max-h-[400px]">
              {(prereqReport?.checks || []).map((check) => {
                const isFail = check.status === 'fail'
                const isWarn = check.status === 'warn'
                
                if (check.status === 'ok') return null; // Only show issues

                return (
                  <div key={check.id} className={`p-4 rounded-lg border ${
                    isFail ? 'bg-red-500/10 border-red-500/30' : 'bg-amber-500/10 border-amber-500/30'
                  }`}>
                    <div className="flex items-start justify-between mb-2">
                      <span className={`font-bold ${isFail ? 'text-red-400' : 'text-amber-400'}`}>
                        {isFail ? '✗' : '⚠'} {check.name}
                      </span>
                      <span className={`text-[10px] uppercase px-2 py-0.5 rounded border ${
                        isFail ? 'text-red-400 border-red-500/50' : 'text-amber-400 border-amber-500/50'
                      }`}>
                        {check.status}
                      </span>
                    </div>
                    <p className="text-slate-300 text-xs leading-relaxed mb-2">
                      {check.details}
                    </p>
                    {check.remediationSteps?.length > 0 && (
                      <div className="mt-2 pt-2 border-t border-white/5">
                        <p className="text-[10px] font-bold text-slate-400 uppercase mb-1">How to fix:</p>
                        <ul className="text-[11px] text-slate-400 list-disc ml-4 space-y-0.5">
                          {check.remediationSteps.map((step, idx) => (
                            <li key={idx}>{step}</li>
                          ))}
                        </ul>
                      </div>
                    )}
                  </div>
                )
              })}
              
              {/* Show OK checks collapsed or simplified */}
              <div className="mt-4 pt-4 border-t border-slate-700">
                <p className="text-xs font-medium text-slate-500 mb-2">Successful Checks:</p>
                <div className="flex flex-wrap gap-2">
                  {(prereqReport?.checks || []).filter(c => c.status === 'ok').map(check => (
                    <span key={check.id} className="text-[10px] bg-slate-900 text-emerald-500/70 px-2 py-1 rounded flex items-center gap-1">
                      ✓ {check.name}
                    </span>
                  ))}
                </div>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Card: Device - next to Status */}
      <div className="bg-slate-800 rounded-lg p-6 flex flex-col justify-center">
        <h3 className="text-xl font-semibold text-white mb-4">Device</h3>
        <div className="flex items-center gap-2">
          <div className={`w-3 h-3 rounded-full ${backupState.deviceConnected ? 'bg-emerald-500' : 'bg-slate-600'}`} />
          <p className={`text-sm ${backupState.deviceConnected ? 'text-emerald-500' : 'text-slate-400'}`}>
            {backupState.deviceConnected
              ? `Connected: ${deviceInfo.name} (${deviceInfo.type} Mode)`
              : 'No device connected'}
          </p>
        </div>
      </div>

      {/* Card: Backup Progress - now after Device */}
      <div className="bg-slate-800 rounded-lg p-6 min-h-[200px] flex flex-col">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-xl font-semibold text-white">Backup Status</h3>
          {backupState.isRunning && (
            <span className="px-3 py-1 text-xs font-medium bg-blue-600 text-white rounded-full">
              Running ⏳
            </span>
          )}
          {backupState.isSuccess && (
            <span className="px-3 py-1 text-xs font-medium bg-emerald-500 text-white rounded-full">
              Completed ✅
            </span>
          )}
          {backupState.isError && (
            <span className="px-3 py-1 text-xs font-medium bg-red-600 text-white rounded-full">
              Failed ❌
            </span>
          )}
        </div>

        {backupState.isRunning && (
          <>
            {/* Live Console-style Status Line */}
            <div className="bg-black/40 rounded border border-slate-700 p-2 mb-4 font-mono text-[11px] text-emerald-400/90 shadow-inner">
              <div className="flex items-center gap-2 overflow-hidden whitespace-nowrap">
                <span className="text-blue-400 font-bold">● LIVE</span>
                <span>
                  [{backupState.summaryStats?.totalFiles || 0} files] 
                  Completed: {backupState.summaryStats?.filesCompleted || 0} | 
                  Skipped: {backupState.summaryStats?.filesSkipped || 0} | 
                  Failed: {backupState.summaryStats?.filesFailed || 0} | 
                  Timeouts: {backupState.summaryStats?.timeoutSkips || 0} 
                  {backupState.summaryStats?.consecutiveSkips > 0 && ` (consecutive: ${backupState.summaryStats.consecutiveSkips})`} |
                  Speed: {backupState.summaryStats?.speed > 0 ? `${backupState.summaryStats.speed.toFixed(2)} ${backupState.summaryStats.speedUnit}` : '0.00 MB/s'}
                </span>
              </div>
            </div>

            {/* Progress Bar - MB-based, more accurate */}
            <div className="w-full bg-slate-700 rounded-full h-6 mb-4 overflow-hidden shadow-inner">
              <div
                className="bg-gradient-to-r from-blue-500 to-blue-400 h-full rounded-full transition-all duration-300 shadow-lg"
                style={{ width: `${backupState.progressMB || backupState.progress || 0}%` }}
              />
              <div className="relative -mt-6 flex items-center justify-center h-6">
                <span className="text-xs font-semibold text-white drop-shadow-md">
                  {backupState.progressMB || backupState.progress || 0}%
                </span>
              </div>
            </div>

            {/* Summary Stats from CLI */}
            {backupState.summaryStats && (backupState.summaryStats.totalFiles > 0 || backupState.summaryStats.filesCompleted > 0 || backupState.summaryStats.filesSkipped > 0) && (
              <div className="bg-slate-900/50 rounded-lg p-3 mb-4 border border-slate-700/50">
                {/* ... grid rows ... */}
                {/* ... existing code ... */}
              </div>
            )}

            {/* Recent Activity Console */}
            {backupState.recentLogs?.length > 0 && (
              <div className="bg-slate-950/80 rounded border border-slate-800 p-3 mb-4 font-mono text-[10px] space-y-1">
                {backupState.recentLogs.map((log, i) => (
                  <div key={i} className={`${i === backupState.recentLogs.length - 1 ? 'text-blue-300' : 'text-slate-500'} truncate`}>
                    <span className="opacity-50 mr-2">{'>'}</span> {log}
                  </div>
                ))}
              </div>
            )}

            {/* Paths - Monospace font with darker background pill */}
            {backupState.sourcePath && (
              <p className="text-sm text-slate-400 mb-2">
                <span className="font-medium text-slate-300">Source:</span>{' '}
                <code className="text-xs font-mono bg-slate-900 text-slate-300 px-2 py-1 rounded">
                  {truncatePath(backupState.sourcePath)}
                </code>
              </p>
            )}
            {(backupState.destPath || destinationPath) && (
              <p className="text-sm text-slate-400 mb-4">
                <span className="font-medium text-slate-300">Destination:</span>{' '}
                <code className="text-xs font-mono bg-slate-900 text-slate-300 px-2 py-1 rounded">
                  {truncatePath(backupState.destPath || destinationPath)}
                </code>
              </p>
            )}

            {/* Cancel Button - Smaller, auto width */}
            <button
              onClick={handleCancelBackup}
              className="mt-auto w-auto px-6 py-2 bg-red-600 text-white rounded-md hover:bg-red-700 transition-colors font-medium"
            >
              Cancel
            </button>
          </>
        )}

        {!backupState.isRunning && !backupState.isSuccess && !backupState.isError && !backupState.lastCompletedTask && (
          <div className="flex-1 flex items-center justify-center text-center px-4">
            <p className="text-slate-400 italic">Ready to fetch some data?<br/>Select a destination and click Start.</p>
          </div>
        )}

        {(backupState.isSuccess || backupState.lastCompletedTask) && !backupState.isRunning && (
          <div className="flex-1 flex flex-col justify-center">
            {/* Dynamic success message based on task type */}
            {(() => {
              const task = backupState.activeTask || backupState.lastCompletedTask
              const taskType = task?.type || 'copy.sync'
              const message = task?.message || ''
              
              let icon = '✓'
              let title = 'Task completed successfully'
              let color = 'text-emerald-500'
              
              if (taskType.includes('verify')) {
                icon = '🔍'
                title = 'Verification complete'
              } else if (taskType.includes('cleanup')) {
                icon = '🧹'
                title = 'Cleanup complete'
              } else if (taskType.includes('copy') || taskType.includes('sync')) {
                icon = '✓'
                title = 'Backup completed successfully'
              }
              
              if (task?.state === 'failed') {
                icon = '❌'
                title = 'Task failed'
                color = 'text-red-500'
              }
              
              return (
                <>
                  <p className={`${color} font-medium text-center mb-2`}>{icon} {title}</p>
                  {message && (
                    <p className="text-slate-400 text-sm text-center mb-4">{message}</p>
                  )}
                </>
              )
            })()}
            
            {/* Final summary stats - show if we have any data */}
            {backupState.summaryStats && (backupState.summaryStats.totalFiles > 0 || backupState.summaryStats.filesCompleted > 0 || backupState.summaryStats.filesSkipped > 0) && (
              <div className="bg-slate-900/50 rounded-lg p-3 border border-emerald-500/20">
                <div className="grid grid-cols-4 gap-2 text-center">
                  <div>
                    <p className="text-[10px] text-slate-500 uppercase">Total</p>
                    <p className="text-lg font-bold text-slate-300">{backupState.summaryStats.totalFiles || '—'}</p>
                  </div>
                  <div>
                    <p className="text-[10px] text-slate-500 uppercase">Completed</p>
                    <p className="text-lg font-bold text-emerald-400">{backupState.summaryStats.filesCompleted || 0}</p>
                  </div>
                  <div>
                    <p className="text-[10px] text-slate-500 uppercase">Skipped</p>
                    <p className={`text-lg font-bold ${backupState.summaryStats.filesSkipped > 0 ? 'text-amber-400' : 'text-slate-500'}`}>
                      {backupState.summaryStats.filesSkipped || 0}
                    </p>
                  </div>
                  <div>
                    <p className="text-[10px] text-slate-500 uppercase">Failed</p>
                    <p className={`text-lg font-bold ${backupState.summaryStats.filesFailed > 0 ? 'text-red-400' : 'text-slate-500'}`}>
                      {backupState.summaryStats.filesFailed || 0}
                    </p>
                  </div>
                </div>
                {backupState.mbProgress && backupState.mbProgress.totalMBCopied > 0 && (
                  <p className="text-center text-sm text-slate-400 mt-2 pt-2 border-t border-slate-700/50">
                    Total: {backupState.mbProgress.totalMBCopied.toFixed(1)} MB transferred
                  </p>
                )}
              </div>
            )}
            
            {/* Show MB stats even if summary stats aren't available */}
            {(!backupState.summaryStats || (backupState.summaryStats.totalFiles === 0 && backupState.summaryStats.filesCompleted === 0)) && 
             backupState.mbProgress && backupState.mbProgress.totalMBCopied > 0 && (
              <div className="bg-slate-900/50 rounded-lg p-3 border border-emerald-500/20 text-center">
                <p className="text-sm text-slate-400">
                  Total: <span className="text-emerald-400 font-semibold">{backupState.mbProgress.totalMBCopied.toFixed(1)} MB</span> transferred
                </p>
              </div>
            )}

            {/* Show discovery stats as fallback */}
            {backupState.discoveryState && backupState.discoveryState.totalFilesFound > 0 && (
              <p className="text-center text-xs text-slate-500 mt-2">
                {backupState.discoveryState.totalFilesFound.toLocaleString()} files discovered across {backupState.discoveryState.completedDirectories} directories
              </p>
            )}
            
            {/* Dismiss button for completed tasks */}
            {backupState.lastCompletedTask && !backupState.isRunning && (
              <button
                onClick={() => backupState.clearLastCompletedTask()}
                className="mt-4 px-4 py-2 text-sm text-slate-400 hover:text-white transition-colors self-center"
              >
                Dismiss
              </button>
            )}
          </div>
        )}

        {backupState.isError && backupState.statusMessage && (
          <div className="flex-1">
            <p className="text-red-600 text-sm">{backupState.statusMessage}</p>
          </div>
        )}
      </div>

      {/* Card: Directory Discovery Progress - only show during active operations */}
      {backupState.discoveryState.isDiscovering && (
        <div className="bg-slate-800 rounded-lg p-6">
          <h3 className="text-xl font-semibold text-white mb-4">
            {backupState.activeTask?.type?.includes('cleanup') ? '🧹 Cleanup Progress' : 
             backupState.activeTask?.type?.includes('verify') ? '🔍 Verification Progress' : 
             'Directory Discovery'}
          </h3>
          
          {/* Current directory being scanned */}
          <div className="mb-4">
            <p className="text-sm text-slate-400 mb-1">Processing:</p>
            <p className="text-sm text-slate-300 font-mono truncate" title={backupState.discoveryState.currentDirectory}>
              {truncatePath(backupState.discoveryState.currentDirectory || backupState.statusMessage, 40)}
            </p>
          </div>
          
          {/* Progress bar for directories */}
          {backupState.discoveryState.totalDirectories > 0 && (
            <div className="w-full bg-slate-700 rounded-full h-4 mb-4">
              <div
                className="bg-blue-500 h-full rounded-full transition-all"
                style={{
                  width: `${(backupState.discoveryState.completedDirectories / backupState.discoveryState.totalDirectories) * 100}%`
                }}
              />
            </div>
          )}
          
          {/* Stats */}
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <p className="text-slate-400">Progress</p>
              <p className="text-white font-semibold">
                {backupState.progress > 0 ? `${backupState.progress.toFixed(0)}%` : 'Processing...'}
              </p>
            </div>
            <div>
              <p className="text-slate-400">Files</p>
              <p className="text-white font-semibold">
                {backupState.discoveryState.totalFilesFound > 0 
                  ? backupState.discoveryState.totalFilesFound.toLocaleString() 
                  : '—'}
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Quick Actions - Full Width */}
      <div className="md:col-span-2 bg-slate-800 rounded-lg p-6 flex flex-col">
        <h3 className="text-xl font-semibold text-white mb-4">Quick Actions</h3>
        
        {/* Destination selector - show above buttons if needed */}
        {!destinationPath && !backupState.destPath && (
          <button
            onClick={handleChooseDestination}
            disabled={!canRunActions || backupState.isRunning}
            className="w-full mb-4 px-4 py-2 border border-slate-600 text-slate-300 rounded-lg hover:bg-slate-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed font-medium"
          >
            Choose Destination
          </button>
        )}

        {/* Buttons in a horizontal row */}
        <div className="flex gap-4 flex-wrap">
          <button
            onClick={handleStartBackup}
            disabled={!canRunActions || isStarting || backupState.isRunning}
            className={`flex-1 min-w-[140px] px-4 py-2 rounded-lg transition-colors font-medium ${
              backupState.isRunning || isStarting
                ? 'bg-slate-700/50 text-slate-500 cursor-not-allowed'
                : 'bg-blue-600 text-white hover:bg-blue-500'
            } disabled:opacity-50 disabled:cursor-not-allowed`}
          >
            {isStarting ? 'Starting...' : backupState.isRunning ? 'Running...' : 'Start Backup'}
          </button>

          <button
            onClick={handleVerifyBackup}
            disabled={!canRunActions || backupState.isRunning || isVerifying || isCleaning || !backupState.sourcePath || (!destinationPath && !backupState.destPath)}
            className="flex-1 min-w-[140px] px-4 py-2 border border-slate-600 text-slate-300 rounded-lg hover:bg-slate-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed font-medium"
          >
            {isVerifying ? 'Verifying...' : 'Verify Backup'}
          </button>

          <button
            onClick={handleCleanupSource}
            disabled={!canRunActions || backupState.isRunning || isVerifying || isCleaning || !backupState.sourcePath || (!destinationPath && !backupState.destPath)}
            className="flex-1 min-w-[140px] px-4 py-2 border border-slate-600 text-slate-300 rounded-lg hover:bg-slate-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed font-medium"
          >
            {isCleaning ? 'Cleaning...' : 'Cleanup Source'}
          </button>
        </div>

        {/* Destination display if set */}
        {(destinationPath || backupState.destPath) && (
          <div className="mt-4 px-4 py-3 bg-slate-700/50 rounded-lg flex items-center justify-between gap-4">
            <div className="flex-1 min-w-0">
              <p className="text-xs text-slate-400 mb-1 uppercase tracking-wider font-semibold">Destination</p>
              <p className="text-sm text-slate-300 truncate font-mono" title={destinationPath || backupState.destPath}>
                {truncatePath(destinationPath || backupState.destPath, 60)}
              </p>
            </div>
            {!backupState.isRunning && (
              <button
                onClick={handleChooseDestination}
                className="px-3 py-1 text-xs bg-slate-600 hover:bg-slate-500 text-white rounded transition-colors whitespace-nowrap border border-slate-500/50"
              >
                Change
              </button>
            )}
          </div>
        )}

        {error && (
          <div className="mt-4 p-3 bg-red-600/20 border border-red-600/50 rounded-lg">
            <p className="text-sm text-red-400">{error}</p>
          </div>
        )}
      </div>

    {/* Hunter Threads - Full Width */}
    <div className="md:col-span-2 bg-slate-800 rounded-lg p-6 flex flex-col">
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-xl font-semibold text-white">Hunter Threads</h3>
        {backupState.isRunning && (
          <div className="flex items-center gap-2">
            <span className="flex h-2 w-2 relative">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-blue-400 opacity-75"></span>
              <span className="relative inline-flex rounded-full h-2 w-2 bg-blue-500"></span>
            </span>
            <span className="text-xs text-blue-400 font-medium">Monitoring {Object.keys(backupState.workers).length} threads</span>
          </div>
        )}
      </div>
      
      {backupState.isRunning ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3">
          {Object.entries(backupState.workers).map(([workerID, worker]) => {
            const isActive = worker.status !== 'idle' && worker.status !== ''
            
            return (
              <div key={workerID} className={`rounded-lg p-3 border transition-all duration-300 ${
                !isActive 
                  ? 'bg-slate-900/30 border-slate-800 opacity-40 scale-[0.98]' 
                  : 'bg-slate-900/80 border-blue-500/40 shadow-lg shadow-blue-500/10 scale-100'
              }`}>
                <div className="flex items-center justify-between mb-2">
                  <div className="flex items-center gap-2">
                    <div className={`w-2 h-2 rounded-full ${!isActive ? 'bg-slate-600' : 'bg-blue-500 animate-pulse shadow-[0_0_8px_rgba(59,130,246,0.5)]'}`} />
                    <span className={`text-xs font-bold uppercase tracking-wider ${!isActive ? 'text-slate-500' : 'text-slate-300'}`}>Hunter {workerID}</span>
                  </div>
                  <span className={`text-[10px] px-2 py-0.5 rounded font-bold ${
                    worker.status === 'copying' ? 'bg-blue-500/30 text-blue-300 border border-blue-500/30' : 
                    worker.status === 'starting' ? 'bg-amber-500/30 text-amber-300 border border-amber-500/30' :
                    worker.status === 'active' ? 'bg-emerald-500/30 text-emerald-300 border border-emerald-500/30' :
                    worker.status === 'failed' ? 'bg-red-500/30 text-red-300 border border-red-500/30' :
                    'bg-slate-800 text-slate-500'
                  }`}>
                    {(worker.status || 'IDLE').toUpperCase()}
                  </span>
                </div>

                {isActive && (
                  <div className="space-y-2">
                    <div className="flex justify-between items-end gap-2">
                      <p className="text-xs text-slate-300 font-mono truncate flex-1" title={worker.fileName || worker.message}>
                        {worker.fileName ? truncatePath(worker.fileName, 25) : (worker.message ? truncatePath(worker.message, 25) : 'Processing...')}
                      </p>
                      {worker.speed && (
                        <span className="text-[10px] text-blue-400 font-bold whitespace-nowrap drop-shadow-sm">{worker.speed}</span>
                      )}
                    </div>

                    <div className="space-y-1">
                      <div className="w-full bg-slate-800 rounded-full h-1.5 overflow-hidden border border-white/5">
                        <div
                          className={`h-full transition-all duration-300 ${
                            worker.status === 'starting' ? 'bg-amber-500' : 'bg-blue-500'
                          }`}
                          style={{ width: `${worker.progress || (worker.status === 'starting' ? 5 : 0)}%` }}
                        />
                      </div>
                      <div className="flex justify-between text-[9px] text-slate-500 font-mono">
                        <span>{worker.progress > 0 ? `${worker.progress.toFixed(1)}%` : (worker.status === 'starting' ? 'Initializing...' : 'Working...')}</span>
                        <span>{worker.fileSize || ''}</span>
                      </div>
                    </div>
                  </div>
                )}
                
                {!isActive && (
                  <div className="h-10 flex items-center justify-center">
                    <span className="text-[10px] text-slate-600 font-mono">STANDBY</span>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      ) : (
        <div className="flex items-center justify-center opacity-30 text-slate-500 py-8">
          <Loader2 size={32} className="mr-3" />
          <p className="text-sm">Threads waiting for deployment...</p>
        </div>
      )}
    </div>
      </div>
    </div>
  )
}

