# Frontend Setup

## Structure

```
frontend/
├── src/
│   ├── pages/
│   │   ├── Home.jsx          # Home dashboard
│   │   ├── Prerequisites.jsx # Prerequisites page
│   │   └── Logs.jsx          # Logs viewer
│   ├── components/
│   │   ├── StatusBadge.jsx   # Status badge component
│   │   └── CheckCard.jsx     # Check card component
│   ├── store.js              # Simple state store
│   ├── App.jsx               # Main app with routing
│   └── main.jsx              # Entry point
├── package.json
├── vite.config.js
└── index.html
```

## Features Implemented

### ✅ Deliverable 3: React Frontend Shell
- React + React Router setup
- Home, Prerequisites, Logs pages
- Simple state store (Context API)
- Event subscriptions (PrereqReport, LogLine)
- TopNav component
- StatusBadge and CheckCard components

### ✅ Deliverable 4: Prereq Gating
- Copy/Verify/Cleanup buttons disabled when `overallStatus === 'fail'`
- Clicking disabled action navigates to `/prereqs` and scrolls to first failed check
- Warning banner shown on Home when prerequisites fail

## Event Subscriptions

The app subscribes to:
- `PrereqReport` - Updates prerequisite status
- `LogLine` - Adds log entries (limited to 2000 entries)

## Service Calls

The app calls:
- `PrereqService.GetPrereqReport()` on startup
- `PrereqService.RefreshNow()` when user clicks "Re-run Checks"

## Next Steps

1. Build frontend: `cd frontend && npm install && npm run build`
2. Run Wails dev: `wails dev` (from repo root)
3. Test event flow and service calls


