import React from 'react'

export default function StatusBadge({ status }) {
  const statusColors = {
    ok: 'bg-emerald-600 text-white',
    fail: 'bg-red-600 text-white',
    warn: 'bg-amber-600 text-white',
  }
  
  const colorClass = statusColors[status] || 'bg-slate-600 text-white'
  
  return (
    <span className={`px-2 py-1 text-xs font-medium rounded ${colorClass}`}>
      {status?.toUpperCase() || 'UNKNOWN'}
    </span>
  )
}


