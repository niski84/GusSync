import React from 'react'
import { useAppStore } from '../store.jsx'

export default function Logs() {
  const store = useAppStore()
  const { logs } = store

  return (
    <div className="min-h-screen bg-slate-950 text-white p-6">
      <h1 className="text-3xl font-bold text-white mb-2">Logs</h1>
      <p className="text-slate-300 mb-6">
        Application logs and events
      </p>

      <div className="bg-slate-800 rounded-lg p-4 font-mono text-sm max-h-[70vh] overflow-y-auto">
        {logs.length === 0 ? (
          <p className="text-slate-300">No logs yet. Events will appear here as they occur.</p>
        ) : (
          logs.map((log, idx) => (
            <div
              key={idx}
              className="py-1 border-b border-slate-700"
              style={{
                color: log.level === 'error' ? '#ef4444' : log.level === 'warn' ? '#f59e0b' : '#e2e8f0',
              }}
            >
              <span className="text-blue-400">[{log.timestamp || new Date().toISOString()}]</span>
              <span className="ml-2 text-slate-400">[{log.level}]</span>
              <span className="ml-2">{log.message}</span>
            </div>
          ))
        )}
      </div>
    </div>
  )
}

