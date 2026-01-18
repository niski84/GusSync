import React from 'react'
import StatusBadge from './StatusBadge'

export default function CheckCard({ check, highlight = false }) {
  return (
    <div 
      className={`bg-slate-800 rounded-lg p-4 mb-4 ${highlight ? 'ring-2 ring-blue-500' : ''}`} 
      id={`check-${check.id}`}
    >
      <h3 className="text-lg font-semibold text-white mb-2 flex items-center gap-2">
        <StatusBadge status={check.status} />
        {check.name}
      </h3>
      <div className="text-slate-300 text-sm mb-3">{check.details}</div>
      {check.remediationSteps && check.remediationSteps.length > 0 && (
        <div className="mt-4 pt-3 border-t border-slate-700">
          <h4 className="text-sm font-semibold text-slate-200 mb-2">How to fix:</h4>
          <ul className="list-disc list-inside space-y-1 text-slate-300 text-sm">
            {check.remediationSteps.map((step, idx) => (
              <li key={idx}>{step}</li>
            ))}
          </ul>
        </div>
      )}
      {check.links && check.links.length > 0 && (
        <div className="links mt-3">
          {check.links.map((link, idx) => (
            <a 
              key={idx} 
              href={link} 
              target="_blank" 
              rel="noopener noreferrer" 
              className="text-blue-400 hover:text-blue-300 mr-3 text-sm"
            >
              Learn more →
            </a>
          ))}
        </div>
      )}
    </div>
  )
}


