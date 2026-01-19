import { useState, useEffect, useMemo } from 'react'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

/**
 * Custom hook to manage backup state and Wails event listeners
 * 
 * Listens to:
 * - job:status -> updates status and isRunning
 * - job:error -> updates status to error
 * - job:log -> updates currentFile and progress (if available)
 * - PrereqReport -> updates deviceConnected from device check
 * - device:connection -> updates deviceConnected (if emitted)
 * 
 * Note: Backend currently emits "job:status" and "job:error"
 * We map these to the desired state variables
 */
export function useBackupState() {
  const [isRunning, setIsRunning] = useState(false)
  const [progress, setProgress] = useState(0)
  const [currentFile, setCurrentFile] = useState('')
  const [status, setStatus] = useState('idle') // "idle" | "running" | "error" | "success"
  const [deviceConnected, setDeviceConnected] = useState(false)
  const [devices, setDevices] = useState([])
  const [statusMessage, setStatusMessage] = useState('')
  const [sourcePath, setSourcePath] = useState('')
  const [destPath, setDestPath] = useState('')
  
  // Discovery state tracking
  const [discoveryState, setDiscoveryState] = useState({
    isDiscovering: false,
    currentDirectory: '',
    totalDirectories: 0,
    completedDirectories: 0,
    timeoutDirectories: 0,
    errorDirectories: 0,
    totalFilesFound: 0,
    directories: [], // Array of directory info
  })

  // Worker state tracking - map of workerID -> worker status
  const [workers, setWorkers] = useState({}) // { [workerID]: { status, fileName, progress, speed, bytesCopied, bytesTotal } }

  // MB-based progress tracking
  const [mbProgress, setMbProgress] = useState({
    totalMBDiscovered: 0,
    totalMBCopied: 0,
    deltaMB: 0, // MB copied in last interval
  })

  // Summary stats from CLI output
  const [summaryStats, setSummaryStats] = useState({
    totalFiles: 0,
    filesCompleted: 0,
    filesSkipped: 0,
    filesFailed: 0,
    speed: 0,
    speedUnit: 'MB/s',
    speedMBps: 0,
  })

  // Recent logs for UI display
  const [recentLogs, setRecentLogs] = useState([]) // Array of string

  useEffect(() => {
    // Runtime check - only set up listeners in Wails environment
    if (!window.runtime) {
      console.warn('[useBackupState] Wails runtime not available - running in browser mode')
      return
    }

    console.log('[useBackupState] Setting up Wails event listeners')

    // Listen to job:status events
    const cleanupStatus = EventsOn('job:status', (data) => {
      console.log('[useBackupState] job:status event:', data)
      
      const state = data.state || 'idle'
      
      setStatus(state)
      setIsRunning(state === 'running')
      setStatusMessage(data.message || '')
      
      // Update paths if provided
      if (data.sourcePath) {
        setSourcePath(data.sourcePath)
        setDeviceConnected(true) // If we have a source path, a device is definitely connected
      }
      if (data.destPath) {
        setDestPath(data.destPath)
      }
      
      // Update progress if provided (assuming 0-100)
      if (data.progress !== undefined) {
        setProgress(data.progress)
      }
      
      // Update current file if provided
      if (data.currentFile) {
        setCurrentFile(data.currentFile)
      }
      
      // Reset on completion/cancellation
      if (state === 'completed' || state === 'cancelled' || state === 'failed') {
        setIsRunning(false)
        if (state === 'completed') {
          setStatus('success')
          setProgress(100)
        } else if (state === 'failed') {
          setStatus('error')
        }
      }
    })

    // Listen to job:error events
    const cleanupError = EventsOn('job:error', (data) => {
      console.error('[useBackupState] job:error event:', data)
      setStatus('error')
      setIsRunning(false)
      setStatusMessage(data.message || 'An error occurred')
    })

    // Listen to job:log events (for progress updates)
    const cleanupLog = EventsOn('job:log', (data) => {
      // console.log('[useBackupState] job:log event:', data)
      // If log contains file info, update currentFile
      if (data.file) {
        setCurrentFile(data.file)
      }
      // If log contains progress, update progress
      if (data.progress !== undefined) {
        setProgress(data.progress)
      }

      // Keep recent logs (last 3)
      if (data.message && !data.message.startsWith('[') && !data.message.includes('W:')) {
        setRecentLogs(prev => {
          const newLogs = [...prev, data.message]
          return newLogs.slice(-3)
        })
      }
    })

    // Listen to job:progress events - contains stats including deltaMB
    const cleanupProgress = EventsOn('job:progress', (data) => {
      console.log('[useBackupState] job:progress event:', data)
      
      // Update MB progress tracking
      if (data.deltaMB !== undefined) {
        setMbProgress(prev => {
          const newTotalCopied = prev.totalMBCopied + (data.deltaMB || 0)
          // console.log(`[useBackupState] Progress Update: Copied ${newTotalCopied.toFixed(2)} MB (+${data.deltaMB.toFixed(2)} MB)`)
          return {
            ...prev,
            deltaMB: data.deltaMB || 0,
            totalMBCopied: newTotalCopied,
          }
        })
      }

      // Update summary stats from CLI output
      setSummaryStats(prev => ({
        totalFiles: data.totalFiles ?? prev.totalFiles,
        filesCompleted: data.filesCompleted ?? prev.filesCompleted,
        filesSkipped: data.filesSkipped ?? prev.filesSkipped,
        filesFailed: data.filesFailed ?? prev.filesFailed,
        speed: data.speed ?? prev.speed,
        speedUnit: data.speedUnit ?? prev.speedUnit,
        speedMBps: data.speedMBps ?? prev.speedMBps,
      }))

      // Update file-based progress as fallback
      if (data.progressFiles !== undefined) {
        const newProgress = parseFloat(data.progressFiles.toFixed(2))
        console.log(`[useBackupState] Updating progress to ${newProgress}% from progressFiles`)
        setProgress(newProgress)
      } else if (data.progress !== undefined) {
        const newProgress = parseFloat(data.progress.toFixed(2))
        console.log(`[useBackupState] Updating progress to ${newProgress}% from progress`)
        setProgress(newProgress)
      }
    })

    // Listen to job:worker events - individual worker status
    const cleanupWorker = EventsOn('job:worker', (data) => {
      // console.log('[useBackupState] job:worker event:', data)
      
      const workerID = data.workerID
      if (workerID !== undefined) {
        const worker = {
          status: data.status || 'idle',
          fileName: data.fileName || '',
          message: data.message || '',
          progress: data.progress || 0,
          speed: data.speed || '',
          bytesCopied: data.bytesCopied || 0,
          bytesTotal: data.bytesTotal || 0,
          fileSize: data.fileSize || '',
        }

        setWorkers(prev => ({
          ...prev,
          [workerID]: worker,
        }))

        // Track total MB discovered - accumulate from file sizes seen in worker status
        if (worker.bytesTotal > 0 && (worker.status === 'copying' || worker.status === 'active' || worker.status === 'starting')) {
          setMbProgress(prev => {
            const fileMB = worker.bytesTotal / (1024 * 1024)
            
            // We need a way to estimate the total MB without double counting.
            // For now, if totalMBDiscovered is 0, initialize it with something
            // to get the progress bar started. As we discover more files, it will grow.
            // A better way is to sum up all file sizes, but we only see them one by one.
            
            // If we have totalFiles from summary stats, we can estimate:
            // average file size * totalFiles
            let estimatedTotal = prev.totalMBDiscovered
            if (estimatedTotal === 0 && summaryStats.totalFiles > 0) {
              estimatedTotal = summaryStats.totalFiles * fileMB // Rough initial estimate
            }
            
            // At minimum, total discovered must be at least what we've copied + what we're currently copying
            const currentMinimum = prev.totalMBCopied + fileMB
            
            return {
              ...prev,
              totalMBDiscovered: Math.max(estimatedTotal, currentMinimum),
            }
          })
        }
      }
    })

    // Listen to backup:progress events (legacy support)
    const cleanupBackupProgress = EventsOn('backup:progress', (data) => {
      console.log('[useBackupState] backup:progress event:', data)
      if (data.progress !== undefined) {
        setProgress(data.progress)
      }
      if (data.currentFile) {
        setCurrentFile(data.currentFile)
      }
    })

    // Listen to backup:status events (if backend emits them)
    const cleanupBackupStatus = EventsOn('backup:status', (data) => {
      console.log('[useBackupState] backup:status event:', data)
      const state = data.state || 'idle'
      setStatus(state)
      setIsRunning(state === 'running')
      if (data.message) {
        setStatusMessage(data.message)
      }
    })

    // Listen to device:connection events (if backend emits them)
    const cleanupDevice = EventsOn('device:connection', (data) => {
      console.log('[useBackupState] device:connection event:', data)
      setDeviceConnected(data.connected || false)
    })

    // Listen to device:list events
    const cleanupDeviceList = EventsOn('device:list', (data) => {
      console.log('[useBackupState] device:list event:', data)
      setDevices(data || [])
      setDeviceConnected(data && data.length > 0)
    })

    // Listen to PrereqReport events to update device connection status
    const cleanupPrereqReport = EventsOn('PrereqReport', (report) => {
      console.log('[useBackupState] PrereqReport event:', report)
      // Check if device is connected based on prereq report
      if (report && report.checks) {
        const deviceCheck = report.checks.find(c => 
          c.id === 'device' || 
          c.id === 'device_connection' || 
          c.name?.toLowerCase().includes('device')
        )
        if (deviceCheck) {
          const isConnected = deviceCheck.status === 'ok'
          console.log('[useBackupState] Device connection status:', isConnected, 'from check:', deviceCheck)
          setDeviceConnected(isConnected)
        }
      }
    })

    // Listen to job:discovery events for directory discovery progress
    const cleanupDiscovery = EventsOn('job:discovery', (data) => {
      console.log('[useBackupState] job:discovery event:', data)
      
      if (data.type === 'directory_scanning') {
        setDiscoveryState(prev => ({
          ...prev,
          isDiscovering: true,
          currentDirectory: data.path || '',
        }))
      } else if (data.type === 'directory_stats') {
        setDiscoveryState(prev => {
          const updatedDirs = [...prev.directories]
          const existingIdx = updatedDirs.findIndex(d => d.path === data.path)
          const dirInfo = {
            path: data.path,
            status: 'completed',
            filesFound: data.filesFound || 0,
            dirsFound: data.dirsFound || 0,
          }
          if (existingIdx >= 0) {
            updatedDirs[existingIdx] = dirInfo
          } else {
            updatedDirs.push(dirInfo)
          }
          
          return {
            ...prev,
            completedDirectories: prev.completedDirectories + 1,
            totalFilesFound: prev.totalFilesFound + (data.filesFound || 0),
            directories: updatedDirs,
          }
        })
      } else if (data.type === 'total_discovered') {
        setDiscoveryState(prev => ({
          ...prev,
          totalFilesFound: data.filesCount || prev.totalFilesFound,
        }))
      } else if (data.type === 'discovery_complete') {
        setDiscoveryState(prev => ({
          ...prev,
          isDiscovering: false,
          currentDirectory: '',
          totalDirectories: data.totalDirs || prev.totalDirectories,
          completedDirectories: data.completedDirs || prev.completedDirectories,
          timeoutDirectories: data.timeoutDirs || prev.timeoutDirs,
          errorDirectories: data.errorDirs || prev.errorDirectories,
        }))
      }
    })

    // Also get initial device connection status and list on mount
    const fetchInitialStatus = async () => {
      // 1. Check devices list
      if (window.go?.services?.DeviceService?.GetDeviceStatus) {
        try {
          const initialDevices = await window.go.services.DeviceService.GetDeviceStatus()
          console.log('[useBackupState] Initial devices fetched:', initialDevices)
          if (initialDevices && initialDevices.length > 0) {
            setDevices(initialDevices)
            setDeviceConnected(true)
          }
        } catch (err) {
          console.warn('[useBackupState] Failed to get initial device status:', err)
        }
      }

      // 2. Check prereq report
      if (window.go?.services?.PrereqService?.GetPrereqReport) {
        try {
          const report = await window.go.services.PrereqService.GetPrereqReport()
          console.log('[useBackupState] Initial prereq report fetched for device status:', report)
          if (report && report.checks) {
            const deviceCheck = report.checks.find(c => 
              c.id === 'device' || 
              c.id === 'device_connection' || 
              c.name?.toLowerCase().includes('device')
            )
            if (deviceCheck) {
              const isConnected = deviceCheck.status === 'ok'
              console.log('[useBackupState] Device connection status from initial report:', isConnected)
              setDeviceConnected(prev => prev || isConnected)
            }
          }
        } catch (err) {
          console.warn('[useBackupState] Failed to get initial prereq report:', err)
        }
      }
    }

    // Wait for bindings to be available
    const waitForBindings = async () => {
      let retries = 0
      while (retries < 10 && (!window.go?.services?.DeviceService || !window.go?.services?.PrereqService)) {
        await new Promise(r => setTimeout(r, 200))
        retries++
      }
      fetchInitialStatus()
    }

    waitForBindings()

    // Cleanup function - remove all listeners on unmount
    return () => {
      console.log('[useBackupState] Cleaning up event listeners')
      cleanupStatus()
      cleanupError()
      cleanupLog()
      cleanupProgress()
      cleanupWorker()
      cleanupBackupProgress()
      cleanupBackupStatus()
      cleanupDevice()
      cleanupDeviceList()
      cleanupPrereqReport()
      cleanupDiscovery()
    }
  }, []) // Empty deps - only set up once on mount

  // Get active workers (not idle)
  const activeWorkers = useMemo(() => {
    return Object.entries(workers).filter(([_, worker]) => worker.status !== 'idle')
  }, [workers])

  // Calculate MB-based progress percentage
  // Use MB progress if we have both discovered and copied MB
  // Otherwise fall back to file-based progress
  const mbProgressPercent = useMemo(() => {
    if (mbProgress.totalMBDiscovered > 0 && mbProgress.totalMBCopied > 0) {
      return Math.min(100, Math.round((mbProgress.totalMBCopied / mbProgress.totalMBDiscovered) * 100))
    }
    // Fallback: if we have MB copied but not discovered, estimate from current active files
    if (mbProgress.totalMBCopied > 0 && activeWorkers.length > 0) {
      const activeFilesMB = activeWorkers.reduce((sum, [_, worker]) => {
        return sum + ((worker.bytesTotal || 0) / (1024 * 1024))
      }, 0)
      const estimatedTotal = mbProgress.totalMBCopied + activeFilesMB
      if (estimatedTotal > 0) {
        return Math.min(100, Math.round((mbProgress.totalMBCopied / estimatedTotal) * 100))
      }
    }
    return progress // Final fallback to file-based progress
  }, [mbProgress.totalMBDiscovered, mbProgress.totalMBCopied, progress, activeWorkers])

  return {
    isRunning,
    progress,
    progressMB: mbProgressPercent, // MB-based progress (more accurate)
    currentFile,
    status,
    deviceConnected,
    devices,
    statusMessage,
    sourcePath,
    destPath,
    discoveryState,
    workers, // All workers
    activeWorkers, // Only active (copying) workers
    mbProgress, // MB tracking: { totalMBDiscovered, totalMBCopied, deltaMB }
    summaryStats, // CLI summary: { totalFiles, filesCompleted, filesSkipped, filesFailed, speed, speedUnit, speedMBps }
    recentLogs, // Last few log messages
    // Helper functions
    isIdle: status === 'idle',
    isSuccess: status === 'success',
    isError: status === 'error',
  }
}
