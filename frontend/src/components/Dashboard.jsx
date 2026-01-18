import React, { useState } from 'react'
import { CheckCircle2, XCircle, Loader2 } from 'lucide-react'
import { useBackupState } from '../hooks/useBackupState'
import { StartBackup, ChooseDestination, CancelCopy } from '../../wailsjs/go/services/CopyService'
import { useAppStore } from '../store.jsx'

export default function Dashboard() {
  const backupState = useBackupState()
  const store = useAppStore()
  const { prereqReport, checkProgress, currentCheckID } = store
  
  const [isStarting, setIsStarting] = useState(false)
  const [destinationPath, setDestinationPath] = useState('')
  const [error, setError] = useState(null)
  
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

  const canRunActions = prereqReport?.overallStatus !== 'fail'
  const systemReady = prereqReport?.overallStatus === 'ok'

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
    if (!canRunActions) {
      return
    }

    // Ensure destination is selected
    let destPath = destinationPath || backupState.destPath
    if (!destPath) {
      destPath = await handleChooseDestination()
      if (!destPath) {
        return // User cancelled
      }
    }

    setIsStarting(true)
    setError(null)

    try {
      await StartBackup('', destPath, 'mount')
      // Don't set isStarting to false here - let events handle it
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

  // Get device info from status or prereq report
  const getDeviceInfo = () => {
    if (backupState.sourcePath) {
      // Extract device name from path like /run/user/1000/gvfs/mtp:host=Xiaomi_Mi_11_Ultra
      const match = backupState.sourcePath.match(/mtp:host=([^/]+)|gphoto2:host=([^/]+)/)
      if (match) {
        return match[1] || match[2] || 'Unknown Device'
      }
      return 'Device Connected'
    }
    return 'No Device'
  }

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
      <div className="bg-slate-800 rounded-lg p-8 flex flex-col items-center justify-center min-h-[200px]">
        {!prereqReport ? (
          <>
            {/* Checking state - Knight Rider scanning effect */}
            <div className="w-full mb-4 relative h-16 bg-slate-900 rounded-lg overflow-hidden">
              {/* Scanning bar - Knight Rider effect */}
              <div className="absolute inset-0 knight-rider-scanner"></div>
            </div>
            <p className="text-2xl font-bold text-blue-400 mb-2">Checking...</p>
            <div className="text-slate-300 text-sm space-y-1">
              <p className="font-medium mb-2">Running system checks:</p>
              <ul className="text-xs space-y-1 ml-4">
                {defaultChecks.map((check) => {
                  const status = checkProgress[check.id] || 'pending'
                  const isCurrent = currentCheckID === check.id
                  const isCompleted = status === 'completed'
                  
                  let className = 'text-slate-400'
                  if (isCompleted) {
                    className = 'text-emerald-400'
                  } else if (isCurrent) {
                    className = 'text-blue-400 font-bold'
                  }
                  
                  return (
                    <li key={check.id} className={className}>
                      {isCompleted && '✓ '}
                      {isCurrent && '→ '}
                      • {check.name}
                    </li>
                  )
                })}
              </ul>
            </div>
          </>
        ) : systemReady ? (
          <>
            <CheckCircle2 size={64} className="text-emerald-500 mb-4" />
            <p className="text-2xl font-bold text-emerald-500 mb-2">OK</p>
            <p className="text-slate-400">System Ready</p>
          </>
        ) : prereqReport?.overallStatus === 'warn' ? (
          <>
            <Loader2 size={64} className="text-amber-500 mb-4 animate-spin" />
            <p className="text-2xl font-bold text-amber-500 mb-2">WARN</p>
            <p className="text-slate-400">Some checks have warnings</p>
          </>
        ) : prereqReport?.overallStatus === 'fail' ? (
          <>
            <XCircle size={64} className="text-red-600 mb-4" />
            <p className="text-2xl font-bold text-red-600 mb-2">FAIL</p>
            <p className="text-slate-400">System checks failed</p>
          </>
        ) : (
          <>
            {/* Fallback - checking or unknown state - Knight Rider effect */}
            <div className="w-full mb-4 relative h-16 bg-slate-900 rounded-lg overflow-hidden">
              <div className="absolute inset-0 knight-rider-scanner"></div>
            </div>
            <p className="text-2xl font-bold text-blue-400 mb-2">Checking...</p>
            <div className="text-slate-300 text-sm space-y-1">
              <p className="font-medium mb-2">
                {prereqReport?.checks?.length > 0 
                  ? `Checking ${prereqReport.checks.length} requirement${prereqReport.checks.length > 1 ? 's' : ''}...`
                  : 'Running system checks...'}
              </p>
              <ul className="text-xs space-y-1 ml-4">
                {defaultChecks.map((check) => {
                  const status = checkProgress[check.id] || 'pending'
                  const isCurrent = currentCheckID === check.id
                  const isCompleted = status === 'completed'
                  
                  let className = 'text-slate-400'
                  if (isCompleted) {
                    className = 'text-emerald-400'
                  } else if (isCurrent) {
                    className = 'text-blue-400 font-bold'
                  }
                  
                  return (
                    <li key={check.id} className={className}>
                      {isCompleted && '✓ '}
                      {isCurrent && '→ '}
                      • {check.name}
                    </li>
                  )
                })}
              </ul>
            </div>
          </>
        )}
      </div>

      {/* Card B: Backup Progress */}
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

            {/* MB Progress Stats */}
            {backupState.mbProgress && backupState.mbProgress.totalMBCopied > 0 && (
              <div className="grid grid-cols-3 gap-2 mb-4 text-sm">
                <div className="text-center">
                  <p className="text-slate-400 text-xs">Copied</p>
                  <p className="text-emerald-400 font-semibold">{backupState.mbProgress.totalMBCopied.toFixed(1)} MB</p>
                </div>
                {backupState.mbProgress.totalMBDiscovered > 0 && (
                  <div className="text-center">
                    <p className="text-slate-400 text-xs">Total</p>
                    <p className="text-slate-300 font-semibold">{backupState.mbProgress.totalMBDiscovered.toFixed(1)} MB</p>
                  </div>
                )}
                <div className="text-center">
                  <p className="text-slate-400 text-xs">Speed</p>
                  <p className="text-blue-400 font-semibold">{backupState.mbProgress.deltaMB > 0 ? `${backupState.mbProgress.deltaMB.toFixed(1)} MB/2s` : '0 MB/2s'}</p>
                </div>
              </div>
            )}

            {/* Active Workers - Show current files being processed */}
            {backupState.activeWorkers && backupState.activeWorkers.length > 0 && (
              <div className="mb-4 space-y-2">
                <p className="text-xs font-medium text-slate-400 mb-2">Active Workers:</p>
                {backupState.activeWorkers.map(([workerID, worker]) => (
                  <div key={workerID} className="bg-slate-900/50 rounded-lg p-2 border border-slate-700">
                    <div className="flex items-center justify-between mb-1">
                      <span className="text-xs font-mono text-blue-400">W{workerID}</span>
                      {worker.speed && (
                        <span className="text-xs text-slate-400">{worker.speed}</span>
                      )}
                    </div>
                    {worker.fileName && (
                      <p className="text-xs text-slate-300 truncate font-mono" title={worker.fileName}>
                        {truncatePath(worker.fileName, 40)}
                      </p>
                    )}
                    {worker.progress > 0 && worker.bytesTotal > 0 && (
                      <div className="mt-1 w-full bg-slate-800 rounded-full h-1.5">
                        <div
                          className="bg-emerald-500 h-full rounded-full transition-all duration-200"
                          style={{ width: `${worker.progress}%` }}
                        />
                      </div>
                    )}
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

        {!backupState.isRunning && !backupState.isSuccess && !backupState.isError && (
          <div className="flex-1 flex items-center justify-center">
            <p className="text-slate-400">No backup in progress</p>
          </div>
        )}

        {backupState.isSuccess && (
          <div className="flex-1 flex items-center justify-center">
            <p className="text-emerald-500 font-medium">Backup completed successfully</p>
          </div>
        )}

        {backupState.isError && backupState.statusMessage && (
          <div className="flex-1">
            <p className="text-red-600 text-sm">{backupState.statusMessage}</p>
          </div>
        )}
      </div>

      {/* Card C: Quick Actions */}
      <div className="bg-slate-800 rounded-lg p-6">
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

        {/* Three buttons in a horizontal row */}
        <div className="flex gap-4">
          <button
            onClick={handleStartBackup}
            disabled={!canRunActions || isStarting || backupState.isRunning}
            className={`flex-1 px-4 py-2 rounded-lg transition-colors font-medium ${
              backupState.isRunning || isStarting
                ? 'bg-slate-700/50 text-slate-500 cursor-not-allowed'
                : 'bg-slate-700 text-white hover:bg-slate-600'
            } disabled:opacity-50 disabled:cursor-not-allowed`}
          >
            {isStarting ? 'Starting...' : backupState.isRunning ? 'Running...' : 'Start Backup'}
          </button>

          <button
            disabled={!canRunActions || backupState.isRunning}
            className="flex-1 px-4 py-2 border border-slate-600 text-slate-300 rounded-lg hover:bg-slate-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed font-medium"
          >
            Verify Backup
          </button>

          <button
            disabled={!canRunActions || backupState.isRunning}
            className="flex-1 px-4 py-2 border border-slate-600 text-slate-300 rounded-lg hover:bg-slate-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed font-medium"
          >
            Cleanup Source
          </button>
        </div>

        {/* Destination display if set */}
        {(destinationPath || backupState.destPath) && (
          <div className="mt-4 px-4 py-2 bg-slate-700/50 rounded-lg">
            <p className="text-xs text-slate-400 mb-1">Destination:</p>
            <p className="text-sm text-slate-300 truncate font-mono" title={destinationPath || backupState.destPath}>
              {truncatePath(destinationPath || backupState.destPath, 35)}
            </p>
          </div>
        )}

        {error && (
          <div className="mt-4 p-3 bg-red-600/20 border border-red-600/50 rounded-lg">
            <p className="text-sm text-red-400">{error}</p>
          </div>
        )}
      </div>

      {/* Card D: Device */}
      <div className="bg-slate-800 rounded-lg p-6">
        <h3 className="text-xl font-semibold text-white mb-4">Device</h3>
        <div className="flex items-center gap-2">
          <div className={`w-3 h-3 rounded-full ${backupState.deviceConnected ? 'bg-emerald-500' : 'bg-slate-600'}`} />
          <p className={`text-sm ${backupState.deviceConnected ? 'text-emerald-500' : 'text-slate-400'}`}>
            {backupState.deviceConnected
              ? `Connected: ${getDeviceInfo()} (MTP Mode)`
              : 'No device connected'}
          </p>
        </div>
      </div>

      {/* Card E: Directory Discovery Progress */}
      <div className="bg-slate-800 rounded-lg p-6">
        <h3 className="text-xl font-semibold text-white mb-4">Directory Discovery</h3>
        
        {backupState.discoveryState.isDiscovering && (
          <>
            {/* Current directory being scanned */}
            <div className="mb-4">
              <p className="text-sm text-slate-400 mb-1">Scanning:</p>
              <p className="text-sm text-slate-300 font-mono truncate" title={backupState.discoveryState.currentDirectory}>
                {truncatePath(backupState.discoveryState.currentDirectory, 40)}
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
                <p className="text-slate-400">Directories</p>
                <p className="text-white font-semibold">
                  {backupState.discoveryState.completedDirectories} / {backupState.discoveryState.totalDirectories || '?'}
                </p>
              </div>
              <div>
                <p className="text-slate-400">Files Found</p>
                <p className="text-white font-semibold">
                  {backupState.discoveryState.totalFilesFound.toLocaleString()}
                </p>
              </div>
              {backupState.discoveryState.timeoutDirectories > 0 && (
                <div>
                  <p className="text-amber-500">Timeouts</p>
                  <p className="text-amber-500">{backupState.discoveryState.timeoutDirectories}</p>
                </div>
              )}
              {backupState.discoveryState.errorDirectories > 0 && (
                <div>
                  <p className="text-red-500">Errors</p>
                  <p className="text-red-500">{backupState.discoveryState.errorDirectories}</p>
                </div>
              )}
            </div>
          </>
        )}
        
        {!backupState.discoveryState.isDiscovering && backupState.discoveryState.totalFilesFound > 0 && (
          <div>
            <p className="text-slate-400 text-sm mb-2">
              Discovery complete: <span className="text-white font-semibold">{backupState.discoveryState.totalFilesFound.toLocaleString()}</span> files found
            </p>
            {backupState.discoveryState.completedDirectories > 0 && (
              <p className="text-slate-400 text-xs">
                Scanned {backupState.discoveryState.completedDirectories} directories
              </p>
            )}
          </div>
        )}
        
        {!backupState.discoveryState.isDiscovering && backupState.discoveryState.totalFilesFound === 0 && (
          <p className="text-slate-400 text-sm">Discovery will start when backup begins</p>
        )}
      </div>
      </div>
    </div>
  )
}

