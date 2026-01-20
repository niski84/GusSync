import { useState, useEffect, useMemo, useRef } from 'react'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

/**
 * Custom hook to manage GusSync tasks and UI state
 * Uses the "task:update" unified event stream as the single source of truth.
 * 
 * Implements startup handshake pattern:
 * 1. On mount, fetch current task state via GetActiveTask()
 * 2. Subscribe to events
 * 3. Ignore out-of-order events using seq number
 */
export function useBackupState() {
  const [activeTask, setActiveTask] = useState(null)
  const [deviceConnected, setDeviceConnected] = useState(false)
  const [devices, setDevices] = useState([])
  const [recentLogs, setRecentLogs] = useState([])
  const [isRunning, setIsRunning] = useState(false)
  const [status, setStatus] = useState('idle')
  const [progress, setProgress] = useState(0)
  const [statusMessage, setStatusMessage] = useState('')
  const [workers, setWorkers] = useState({})
  const [sourcePath, setSourcePath] = useState('')
  const [destPath, setDestPath] = useState('')
  const [lastCompletedTask, setLastCompletedTask] = useState(null)
  const [discoveryState, setDiscoveryState] = useState({
    isDiscovering: false,
    totalFilesFound: 0,
    completedDirectories: 0,
    currentDirectory: '',
    totalDirectories: 0,
    timeoutDirectories: 0,
    errorDirectories: 0
  })

  // Track last seen sequence number for out-of-order protection
  const lastSeqRef = useRef(0)

  // Helper function to process task updates (used by both initial fetch and events)
  const processTaskUpdate = (task) => {
    if (!task) return false

    // Out-of-order protection: ignore events with seq <= lastSeqSeen
    const taskSeq = task.seq || 0
    if (taskSeq > 0 && taskSeq <= lastSeqRef.current) {
      console.log('[useBackupState] Ignoring out-of-order event, seq:', taskSeq, 'lastSeen:', lastSeqRef.current)
      return false
    }
    lastSeqRef.current = taskSeq

    setActiveTask(task)
    
    // Map to legacy fields for backward compatibility with Dashboard.jsx
    const stateMap = {
      'queued': 'running',
      'running': 'running',
      'succeeded': 'success',
      'failed': 'error',
      'canceled': 'idle'
    }
    
    const mappedStatus = stateMap[task.state] || 'idle'
    setStatus(mappedStatus)
    setIsRunning(task.state === 'running' || task.state === 'queued')
    setProgress(task.progress?.percent || 0)
    setStatusMessage(task.message || '')

    // Update paths if available in task params
    if (task.params?.sourcePath) setSourcePath(task.params.sourcePath)
    if (task.params?.destPath) setDestPath(task.params.destPath)
    if (task.params?.sourceRoot) setSourcePath(task.params.sourceRoot)
    if (task.params?.destRoot) setDestPath(task.params.destRoot)

    // Update discovery state from task progress
    if (task.progress?.phase === 'scanning' || task.progress?.phase === 'starting') {
      setDiscoveryState({
        isDiscovering: true,
        totalFilesFound: task.progress?.current || 0,
        completedDirectories: 0,
        currentDirectory: task.message || '',
        totalDirectories: 0,
        timeoutDirectories: 0,
        errorDirectories: 0
      })
    } else if (task.progress?.phase === 'copying' || task.progress?.phase === 'finishing' || 
               task.progress?.phase === 'verifying' || task.progress?.phase === 'cleaning') {
      setDiscoveryState(prev => ({
        ...prev,
        isDiscovering: false,
        totalFilesFound: task.progress?.total || prev.totalFilesFound
      }))
    }
    
    // Reset discovery state when task completes or fails
    if (task.state === 'succeeded' || task.state === 'failed' || task.state === 'canceled') {
      setDiscoveryState(prev => ({
        ...prev,
        isDiscovering: false
      }))
      // Save the completed task so we can show the report
      setLastCompletedTask({
        ...task,
        completedAt: new Date().toISOString()
      })
    }

    if (task.workers) {
      // Map new workers model to legacy format for Dashboard.jsx
      const mappedWorkers = {}
      Object.entries(task.workers).forEach(([id, workerStatus]) => {
        mappedWorkers[id] = {
          status: workerStatus.includes('Copying') ? 'copying' : (workerStatus.includes('Starting') ? 'starting' : 'active'),
          message: workerStatus,
          fileName: workerStatus.split(': ')[1] || '',
          progress: workerStatus.includes('%') ? parseFloat(workerStatus.match(/(\d+\.?\d*)%/)?.[1] || 0) : 0
        }
      })
      setWorkers(mappedWorkers)
    }

    if (task.logLine) {
      setRecentLogs(prev => {
        const newLogs = [...prev, task.logLine]
        return newLogs.slice(-10) // Show more in unified model
      })
    }

    return true
  }

  useEffect(() => {
    if (!window.runtime) return

    console.log('[useBackupState] Setting up unified task listener with startup handshake')

    // STARTUP HANDSHAKE: Fetch current task state before subscribing to events
    const fetchActiveTask = async () => {
      if (window.go?.services?.JobManager?.GetActiveTask) {
        try {
          const task = await window.go.services.JobManager.GetActiveTask()
          if (task) {
            console.log('[useBackupState] Startup handshake - got active task:', task)
            processTaskUpdate(task)
          } else {
            console.log('[useBackupState] Startup handshake - no active task')
          }
        } catch (e) {
          console.warn('[useBackupState] Failed to fetch active task:', e)
        }
      }
    }
    fetchActiveTask()

    // Unified Task Listener (now with seq protection via processTaskUpdate)
    const cleanupTask = EventsOn('task:update', (task) => {
      console.log('[useBackupState] task:update:', task)
      processTaskUpdate(task)
    })

    // Device List Listener (keeps device status updated independently of tasks)
    const cleanupDeviceList = EventsOn('device:list', (data) => {
      setDevices(data || [])
      setDeviceConnected(data && data.length > 0)
    })

    // Prereq Report Listener (for device connection status)
    const cleanupPrereq = EventsOn('PrereqReport', (report) => {
      if (report && report.checks) {
        const deviceCheck = report.checks.find(c => 
          c.id === 'device' || c.id === 'device_connection'
        )
        if (deviceCheck) {
          setDeviceConnected(deviceCheck.status === 'ok')
        }
      }
    })

    // Initial check for devices and config
    const fetchInitial = async () => {
      // 1. Get devices
      if (window.go?.services?.DeviceService?.GetDeviceStatus) {
        try {
          const devs = await window.go.services.DeviceService.GetDeviceStatus()
          setDevices(devs || [])
          setDeviceConnected(devs && devs.length > 0)
          if (devs && devs.length > 0 && !sourcePath) {
            setSourcePath(devs[0].path)
          }
        } catch (e) { console.warn(e) }
      }

      // 2. Get destination path from config
      if (window.go?.services?.ConfigService?.GetConfig) {
        try {
          const cfg = await window.go.services.ConfigService.GetConfig()
          if (cfg?.destinationPath) {
            setDestPath(cfg.destinationPath)
          }
        } catch (e) { console.warn(e) }
      }
    }
    
    fetchInitial()

    return () => {
      cleanupTask()
      cleanupDeviceList()
      cleanupPrereq()
    }
  }, [])

  // Action: List all tasks
  const listTasks = async () => {
    if (window.go?.services?.JobManager?.ListTasks) {
      try {
        return await window.go.services.JobManager.ListTasks()
      } catch (e) {
        console.warn('Failed to list tasks:', e)
        return []
      }
    }
    return []
  }

  // Action: Cancel task
  const cancelTask = async (taskId) => {
    if (window.go?.services?.JobManager?.CancelTask) {
      try {
        await window.go.services.JobManager.CancelTask(taskId)
      } catch (e) {
        console.warn('Failed to cancel task:', e)
      }
    }
  }

  return {
    activeTask,
    isRunning,
    progress,
    progressMB: progress, // Alias for MB progress in new model
    status,
    deviceConnected,
    devices,
    sourcePath,
    destPath,
    statusMessage,
    recentLogs,
    workers,
    discoveryState,
    lastCompletedTask,
    listTasks,
    cancelTask,
    clearLastCompletedTask: () => setLastCompletedTask(null),
    // Helper fields
    isIdle: status === 'idle' || status === 'ready',
    isSuccess: status === 'success',
    isError: status === 'error',
    summaryStats: activeTask ? {
      totalFiles: activeTask.progress.total,
      filesCompleted: activeTask.progress.current,
      speed: activeTask.progress.rate,
      speedUnit: 'MB/s'
    } : null
  }
}
