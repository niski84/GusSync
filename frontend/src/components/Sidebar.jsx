import React from 'react'
import { Link, useLocation } from 'react-router-dom'
import { ClipboardCheck, FileText, Home } from 'lucide-react'

export default function Sidebar() {
  const location = useLocation()
  
  const isActive = (path) => {
    if (path === '/') {
      return location.pathname === '/'
    }
    return location.pathname.startsWith(path)
  }

  return (
    <div className="w-[250px] h-screen bg-slate-900 flex flex-col fixed left-0 top-0 border-r border-slate-800">
      {/* Header */}
      <div className="p-6 border-b border-slate-800">
        <h1 className="text-xl font-bold text-white">GusSync</h1>
        <p className="text-sm text-slate-400 mt-1">v1.0.0</p>
      </div>

      {/* Navigation */}
      <nav className="flex-1 p-4">
        <ul className="space-y-2">
          <li>
            <Link
              to="/"
              className={`flex items-center gap-3 px-4 py-3 rounded-lg transition-colors ${
                isActive('/')
                  ? 'bg-slate-800 text-white shadow-sm'
                  : 'text-slate-400 hover:bg-slate-800/50 hover:text-slate-300'
              }`}
            >
              <div className="flex items-center gap-3 flex-1">
                <Home size={20} />
                <span className="font-medium">Home</span>
              </div>
              {isActive('/') && (
                <div className="w-2.5 h-2.5 rounded-full bg-emerald-500 shadow-sm shadow-emerald-500/50"></div>
              )}
            </Link>
          </li>

          <li>
            <Link
              to="/prereqs"
              className={`flex items-center gap-3 px-4 py-3 rounded-lg transition-colors ${
                isActive('/prereqs')
                  ? 'bg-slate-800 text-white'
                  : 'text-slate-400 hover:bg-slate-800/50 hover:text-slate-300'
              }`}
            >
              <ClipboardCheck size={20} />
              <span className="font-medium">Prerequisites</span>
            </Link>
          </li>

          <li>
            <Link
              to="/logs"
              className={`flex items-center gap-3 px-4 py-3 rounded-lg transition-colors ${
                isActive('/logs')
                  ? 'bg-slate-800 text-white'
                  : 'text-slate-400 hover:bg-slate-800/50 hover:text-slate-300'
              }`}
            >
              <FileText size={20} />
              <span className="font-medium">Logs</span>
            </Link>
          </li>
        </ul>
      </nav>

      {/* Footer */}
      <div className="p-4 border-t border-slate-800">
        <p className="text-xs text-slate-500 text-center">
          Digs deep. Fetches everything. Never lets go.
        </p>
      </div>
    </div>
  )
}

