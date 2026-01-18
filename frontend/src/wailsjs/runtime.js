// Wails v2 runtime bridge
// This file provides the runtime API if not already available
// Wails will inject the real runtime, but this provides fallback

let runtime = null

// Try to get runtime from window
if (typeof window !== 'undefined') {
  if (window.wails && window.wails.runtime) {
    runtime = window.wails.runtime
  } else if (window.runtime) {
    runtime = window.runtime
  }
}

// Export runtime for use in React
export { runtime }

// Event subscription helpers (if runtime available)
export function EventsOn(eventName, callback) {
  if (runtime && runtime.EventsOn) {
    return runtime.EventsOn(eventName, callback)
  }
  console.warn(`[WailsBridge] EventsOn called but runtime not available: ${eventName}`)
}

export function EventsOff(eventName, callback) {
  if (runtime && runtime.EventsOff) {
    return runtime.EventsOff(eventName, callback)
  }
}

export function EventsEmit(eventName, data) {
  if (runtime && runtime.EventsEmit) {
    return runtime.EventsEmit(eventName, data)
  }
  console.warn(`[WailsBridge] EventsEmit called but runtime not available: ${eventName}`)
}


