// Wails bridge - expose Wails runtime to window for frontend use
// This file is loaded before React app starts

if (window.wails && window.wails.runtime) {
  // Wails v2 exposes runtime directly
  window.runtime = window.wails.runtime
} else if (window.runtime) {
  // Already available
} else {
  console.warn('[WailsBridge] Wails runtime not found. Make sure Wails is properly initialized.')
}

// Expose services to window for React to use
// These are automatically injected by Wails v2
// window.PrereqService, window.DeviceService, etc. will be available


