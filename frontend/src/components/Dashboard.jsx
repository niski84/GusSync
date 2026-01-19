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

  const handleVerifyBackup = async () => {
    const sourcePath = backupState.sourcePath
    const destPath = destinationPath || backupState.destPath

    if (!sourcePath || !destPath) {
      setError('Please set source and destination paths before verifying')
      return
    }

    setIsVerifying(true)
    setError(null)

    try {
      await StartVerify({
        sourcePath: sourcePath,
        destPath: destPath,
        level: 'full', // Use 'full' for complete verification, 'quick' for faster check
      })
      // Verification started - backend will emit events for progress
      console.log('[Dashboard] Verify started')
    } catch (error) {
      console.error('[Dashboard] Failed to start verification:', error)
      setError(`Failed to start verification: ${error.message || error}`)
      setIsVerifying(false)
    }
  }

  const handleCleanupSource = async () => {
    const sourcePath = backupState.sourcePath
    const destPath = destinationPath || backupState.destPath

    if (!sourcePath || !destPath) {
      setError('Please set source and destination paths before cleanup')
      return
    }

    setIsCleaning(true)
    setError(null)

    try {
      await StartCleanup({
        sourceRoot: sourcePath,
        destRoot: destPath,
        stateFiles: [], // Empty array means auto-detect
        processBoth: true, // Process both mount and adb state files if available
      })
      // Cleanup started - backend will emit events for progress
      console.log('[Dashboard] Cleanup started')
    } catch (error) {
      console.error('[Dashboard] Failed to start cleanup:', error)
      setError(`Failed to start cleanup: ${error.message || error}`)
      setIsCleaning(false)
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
              ? `Connected: ${getDeviceInfo()} (MTP Mode)`
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
                {/* File counts row */}
                <div className="grid grid-cols-4 gap-2 text-center mb-3">
                  <div>
                    <p className="text-[10px] text-slate-500 uppercase tracking-wider">Total</p>
                    <p className="text-lg font-bold text-slate-300">{backupState.summaryStats.totalFiles || '—'}</p>
                  </div>
                  <div>
                    <p className="text-[10px] text-slate-500 uppercase tracking-wider">Done</p>
                    <p className="text-lg font-bold text-emerald-400">{backupState.summaryStats.filesCompleted || 0}</p>
                  </div>
                  <div>
                    <p className="text-[10px] text-slate-500 uppercase tracking-wider">Skipped</p>
                    <p className={`text-lg font-bold ${backupState.summaryStats.filesSkipped > 0 ? 'text-amber-400' : 'text-slate-500'}`}>
                      {backupState.summaryStats.filesSkipped || 0}
                    </p>
                  </div>
                  <div>
                    <p className="text-[10px] text-slate-500 uppercase tracking-wider">Failed</p>
                    <p className={`text-lg font-bold ${backupState.summaryStats.filesFailed > 0 ? 'text-red-400' : 'text-slate-500'}`}>
                      {backupState.summaryStats.filesFailed || 0}
                    </p>
                  </div>
                </div>
                
                {/* Speed and MB row */}
                <div className="grid grid-cols-3 gap-2 text-center pt-2 border-t border-slate-700/50">
                  <div>
                    <p className="text-[10px] text-slate-500 uppercase tracking-wider">Speed</p>
                    <p className="text-sm font-semibold text-blue-400">
                      {backupState.summaryStats.speed > 0 
                        ? `${backupState.summaryStats.speed.toFixed(1)} ${backupState.summaryStats.speedUnit}`
                        : '—'}
                    </p>
                  </div>
                  <div>
                    <p className="text-[10px] text-slate-500 uppercase tracking-wider">Copied</p>
                    <p className="text-sm font-semibold text-emerald-400">
                      {backupState.mbProgress.totalMBCopied > 0 
                        ? `${backupState.mbProgress.totalMBCopied.toFixed(1)} MB`
                        : '—'}
                    </p>
                  </div>
                  <div>
                    <p className="text-[10px] text-slate-500 uppercase tracking-wider">Delta</p>
                    <p className="text-sm font-semibold text-slate-300">
                      {backupState.mbProgress.deltaMB > 0 
                        ? `+${backupState.mbProgress.deltaMB.toFixed(1)} MB`
                        : '—'}
                    </p>
                  </div>
                </div>
              </div>
            )}
            
            {/* Fallback: MB Progress Stats (when summary stats not available) */}
            {(!backupState.summaryStats || (backupState.summaryStats.totalFiles === 0 && backupState.summaryStats.filesCompleted === 0 && backupState.summaryStats.filesSkipped === 0)) && 
             backupState.mbProgress && backupState.mbProgress.totalMBCopied > 0 && (
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
          <div className="flex-1 flex items-center justify-center text-center px-4">
            <p className="text-slate-400 italic">Ready to fetch some data?<br/>Select a destination and click Start.</p>
          </div>
        )}

        {backupState.isSuccess && (
          <div className="flex-1 flex flex-col justify-center">
            <p className="text-emerald-500 font-medium text-center mb-4">✓ Backup completed successfully</p>
            
            {/* Final summary stats - show if we have any data */}
            {backupState.summaryStats && (backupState.summaryStats.totalFiles > 0 || backupState.summaryStats.filesCompleted > 0 || backupState.summaryStats.filesSkipped > 0) && (
              <div className="bg-slate-900/50 rounded-lg p-3 border border-emerald-500/20">
                <div className="grid grid-cols-4 gap-2 text-center">
                  <div>
                    <p className="text-[10px] text-slate-500 uppercase">Total</p>
                    <p className="text-lg font-bold text-slate-300">{backupState.summaryStats.totalFiles || '—'}</p>
                  </div>
                  <div>
                    <p className="text-[10px] text-slate-500 uppercase">Copied</p>
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
          </div>
        )}

        {backupState.isError && backupState.statusMessage && (
          <div className="flex-1">
            <p className="text-red-600 text-sm">{backupState.statusMessage}</p>
          </div>
        )}
      </div>

      {/* Card: Directory Discovery Progress */}
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
          <div className="mt-4 px-4 py-2 bg-slate-700/50 rounded-lg">
            <p className="text-xs text-slate-400 mb-1">Destination:</p>
            <p className="text-sm text-slate-300 truncate font-mono" title={destinationPath || backupState.destPath}>
              {truncatePath(destinationPath || backupState.destPath, 60)}
            </p>
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
        <h3 className="text-xl font-semibold text-white mb-4">Hunter Threads</h3>
        
        {backupState.isRunning ? (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3">
            {Object.entries(backupState.workers).map(([workerID, worker]) => (
              <div key={workerID} className={`rounded-lg p-3 border ${
                worker.status === 'idle' ? 'bg-slate-900/30 border-slate-800 opacity-50' : 'bg-slate-900/80 border-blue-500/30'
              }`}>
                <div className="flex items-center justify-between mb-2">
                  <div className="flex items-center gap-2">
                    <div className={`w-2 h-2 rounded-full ${worker.status === 'idle' ? 'bg-slate-600' : 'bg-blue-500 animate-pulse'}`} />
                    <span className="text-xs font-bold text-slate-400 uppercase tracking-wider">Hunter {workerID}</span>
                  </div>
                  <span className={`text-[10px] px-2 py-0.5 rounded ${
                    worker.status === 'copying' ? 'bg-blue-500/20 text-blue-400' : 
                    worker.status === 'starting' ? 'bg-amber-500/20 text-amber-400' :
                    worker.status === 'idle' ? 'bg-slate-800 text-slate-500' : 'bg-slate-800 text-slate-300'
                  }`}>
                    {worker.status.toUpperCase()}
                  </span>
                </div>

                {worker.status !== 'idle' && (
                  <div className="space-y-2">
                    <div className="flex justify-between items-end gap-2">
                      <p className="text-xs text-slate-300 font-mono truncate flex-1" title={worker.fileName}>
                        {worker.fileName ? truncatePath(worker.fileName, 25) : worker.message}
                      </p>
                      {worker.speed && (
                        <span className="text-[10px] text-blue-400 font-bold whitespace-nowrap">{worker.speed}</span>
                      )}
                    </div>

                    {worker.status === 'copying' && worker.progress > 0 && (
                      <div className="space-y-1">
                        <div className="w-full bg-slate-800 rounded-full h-1.5 overflow-hidden">
                          <div
                            className="bg-blue-500 h-full transition-all duration-300"
                            style={{ width: `${worker.progress}%` }}
                          />
                        </div>
                        <div className="flex justify-between text-[9px] text-slate-500 font-mono">
                          <span>{worker.progress.toFixed(1)}%</span>
                          <span>{worker.fileSize}</span>
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
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

