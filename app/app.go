package app

import (
	"context"
	"embed"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"GusSync/app/services"
	"GusSync/internal/adapters/api"
)

//go:embed all:frontend_dist
var assets embed.FS

// App struct holds the application state and services
type App struct {
	ctx            context.Context
	prereqService  *services.PrereqService
	deviceService  *services.DeviceService
	copyService    *services.CopyService
	verifyService  *services.VerifyService
	cleanupService *services.CleanupService
	logService     *services.LogService
	jobManager     *services.JobManager
	systemService  *services.SystemService
	configService  *services.ConfigService
	apiServer      *api.Server
	logger         *log.Logger
}

// NewApp creates a new App instance
func NewApp() *App {
	// Create logger
	logger := log.New(os.Stderr, "[GusSync] ", log.LstdFlags|log.Lshortfile)

	return &App{
		logger: logger,
	}
}

// OnStartup is called when the app starts (called by Wails after frontend loads)
func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx
	logger := log.New(os.Stderr, "[GusSync] ", log.LstdFlags|log.Lshortfile)
	
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logger.Printf("[TIMING %s] [App] OnStartup: ENTRY - OnStartup called by Wails (frontend should be loaded now)", timestamp)

	// Initialize config service first
	configStart := time.Now()
	configService, err := services.NewConfigService(logger)
	a.configService = configService
	configDuration := time.Since(configStart)
	if err != nil {
		logger.Printf("[TIMING %s] [App] OnStartup: Config service initialization FAILED (took %v): %v", time.Now().Format("2006-01-02 15:04:05.000"), configDuration, err)
		// Continue without config service
	} else {
		logger.Printf("[TIMING %s] [App] OnStartup: Config service initialized (took %v)", time.Now().Format("2006-01-02 15:04:05.000"), configDuration)
	}

	// Restore window geometry if available
	if a.configService != nil {
		cfg := a.configService.GetConfig()
		if cfg.WindowWidth > 0 && cfg.WindowHeight > 0 {
			logger.Printf("[App] OnStartup: Restoring window geometry: %dx%d at (%d,%d)", cfg.WindowWidth, cfg.WindowHeight, cfg.WindowX, cfg.WindowY)
			runtime.WindowSetSize(ctx, cfg.WindowWidth, cfg.WindowHeight)
			runtime.WindowSetPosition(ctx, cfg.WindowX, cfg.WindowY)
		} else {
			runtime.WindowCenter(ctx)
		}
	} else {
		runtime.WindowCenter(ctx)
	}

	startTime := time.Now()

	// Update service contexts using the Wails context provided in OnStartup
	// CRITICAL: We MUST NOT replace the service instances because Wails has already 
	// bound to the memory addresses of the instances created in Run().
	// Instead, we call SetContext() on each existing instance.
	serviceStart := time.Now()
	a.jobManager.SetContext(ctx)
	jobDuration := time.Since(serviceStart)
	logger.Printf("[TIMING %s] [App] OnStartup: JobManager context updated (took %v)", time.Now().Format("2006-01-02 15:04:05.000"), jobDuration)
	
	prereqStart := time.Now()
	a.prereqService.SetContext(ctx)
	prereqDuration := time.Since(prereqStart)
	logger.Printf("[TIMING %s] [App] OnStartup: PrereqService context updated (took %v)", time.Now().Format("2006-01-02 15:04:05.000"), prereqDuration)
	
	deviceStart := time.Now()
	a.deviceService.SetContext(ctx)
	deviceDuration := time.Since(deviceStart)
	logger.Printf("[TIMING %s] [App] OnStartup: DeviceService context updated (took %v)", time.Now().Format("2006-01-02 15:04:05.000"), deviceDuration)
	
	copyStart := time.Now()
	a.copyService.SetContext(ctx)
	if configService != nil {
		a.copyService.SetConfig(configService)
	}
	copyDuration := time.Since(copyStart)
	logger.Printf("[TIMING %s] [App] OnStartup: CopyService context updated (took %v)", time.Now().Format("2006-01-02 15:04:05.000"), copyDuration)
	
	verifyStart := time.Now()
	a.verifyService.SetContext(ctx)
	verifyDuration := time.Since(verifyStart)
	logger.Printf("[TIMING %s] [App] OnStartup: VerifyService context updated (took %v)", time.Now().Format("2006-01-02 15:04:05.000"), verifyDuration)
	
	cleanupStart := time.Now()
	a.cleanupService.SetContext(ctx)
	cleanupDuration := time.Since(cleanupStart)
	logger.Printf("[TIMING %s] [App] OnStartup: CleanupService context updated (took %v)", time.Now().Format("2006-01-02 15:04:05.000"), cleanupDuration)
	
	logStart := time.Now()
	a.logService.SetContext(ctx)
	logDuration := time.Since(logStart)
	logger.Printf("[TIMING %s] [App] OnStartup: LogService context updated (took %v)", time.Now().Format("2006-01-02 15:04:05.000"), logDuration)

	systemStart := time.Now()
	a.systemService.SetContext(ctx)
	systemDuration := time.Since(systemStart)
	logger.Printf("[TIMING %s] [App] OnStartup: SystemService context updated (took %v)", time.Now().Format("2006-01-02 15:04:05.000"), systemDuration)

	totalServiceDuration := time.Since(startTime)
	logger.Printf("[TIMING %s] [App] OnStartup: All services updated (took %v total)", time.Now().Format("2006-01-02 15:04:05.000"), totalServiceDuration)

	// Start device polling for immediate UI updates when phone is plugged in
	a.deviceService.StartPolling(ctx)

	// Run prerequisite checks immediately and cache results
	logger.Printf("[TIMING %s] [App] OnStartup: Running prerequisite checks...", time.Now().Format("2006-01-02 15:04:05.000"))
	go func() {
		checkStart := time.Now()
		report := a.prereqService.GetPrereqReport()
		checkDuration := time.Since(checkStart)
		logger.Printf("[TIMING %s] [App] OnStartup: Prerequisite checks completed (took %v) - report cached", time.Now().Format("2006-01-02 15:04:05.000"), checkDuration)
		logger.Printf("[App] OnStartup: Overall status: %s", report.OverallStatus)
	}()

	// Start window position monitor to save position on move/resize
	go a.monitorWindowPosition(ctx)

	// Start API server if enabled via environment variable
	// Set GUSSYNC_API_PORT to enable (e.g., GUSSYNC_API_PORT=8080)
	if apiPort := os.Getenv("GUSSYNC_API_PORT"); apiPort != "" {
		port, err := strconv.Atoi(apiPort)
		if err != nil {
			logger.Printf("[App] OnStartup: Invalid API port '%s': %v", apiPort, err)
		} else {
			a.startAPIServer(ctx, port, logger)
		}
	}

	totalDuration := time.Since(startTime)
	logger.Printf("[TIMING %s] [App] OnStartup: EXIT - Total startup time: %v", time.Now().Format("2006-01-02 15:04:05.000"), totalDuration)
}

// OnBeforeClose is called when the window is about to close (before it's destroyed)
// This is the correct place to save window geometry because the window is still visible
func (a *App) OnBeforeClose(ctx context.Context) bool {
	a.logger.Printf("[App] OnBeforeClose: Window is about to close...")

	// Save window geometry - the window is still available at this point
	if a.configService != nil && a.ctx != nil {
		x, y := runtime.WindowGetPosition(a.ctx)
		w, h := runtime.WindowGetSize(a.ctx)
		a.logger.Printf("[App] OnBeforeClose: Got window geometry: %dx%d at (%d,%d)", w, h, x, y)
		if w > 0 && h > 0 {
			a.logger.Printf("[App] OnBeforeClose: Saving window geometry: %dx%d at (%d,%d)", w, h, x, y)
			if err := a.configService.SetWindowGeometry(x, y, w, h); err != nil {
				a.logger.Printf("[App] OnBeforeClose: Failed to save window geometry: %v", err)
			}
		} else {
			a.logger.Printf("[App] OnBeforeClose: Skipping save of invalid window geometry: %dx%d", w, h)
		}
	}

	// Return false to allow the window to close (true would prevent closing)
	return false
}

// OnShutdown is called when the app is shutting down (after window is closed)
func (a *App) OnShutdown(ctx context.Context) {
	a.logger.Printf("[App] OnShutdown: Shutting down...")

	// Cancel any running jobs
	if a.jobManager != nil {
		if err := a.jobManager.CancelJob(); err != nil {
			a.logger.Printf("[App] OnShutdown: Error cancelling job: %v", err)
		}
	}

	a.logger.Printf("[App] OnShutdown: Shutdown complete")
}

// startAPIServer initializes and starts the HTTP API server
func (a *App) startAPIServer(ctx context.Context, port int, logger *log.Logger) {
	logger.Printf("[App] Starting API server on port %d", port)

	// Create the API server with the core job manager
	coreJobManager := a.jobManager.GetCoreJobManager()

	a.apiServer = api.NewServer(port, logger, coreJobManager,
		// Provider for prerequisites
		api.WithPrereqProvider(func() interface{} {
			return a.prereqService.GetPrereqReport()
		}),
		// Provider for devices
		api.WithDeviceProvider(func() interface{} {
			devices, err := a.deviceService.GetDeviceStatus()
			if err != nil {
				return map[string]interface{}{
					"devices":   []interface{}{},
					"connected": false,
					"error":     err.Error(),
				}
			}
			return map[string]interface{}{
				"devices":   devices,
				"connected": len(devices) > 0,
			}
		}),
		// Provider for config
		api.WithConfigProvider(func() interface{} {
			if a.configService != nil {
				return a.configService.GetConfig()
			}
			return nil
		}),
		// Function to start a copy operation
		api.WithStartCopyFunc(func(reqCtx context.Context, req api.StartCopyRequest) (string, error) {
			// Use config values if not provided in request
			dest := req.DestinationPath
			if dest == "" && a.configService != nil {
				cfg := a.configService.GetConfig()
				dest = cfg.DestinationPath
			}
			// Use default mode "smart"
			return a.copyService.StartBackup("", dest, "smart")
		}),
	)

	// Register the API server as an additional event emitter
	// This allows SSE clients to receive job updates alongside the Wails UI
	coreJobManager.AddEmitter(a.apiServer)

	// Start the API server in background
	a.apiServer.StartBackground(ctx)
}

// monitorWindowPosition watches for window position/size changes and saves them
// This ensures the position is saved even if the app is killed unexpectedly
func (a *App) monitorWindowPosition(ctx context.Context) {
	// Wait a bit for window to stabilize after startup
	time.Sleep(2 * time.Second)

	var lastX, lastY, lastW, lastH int
	var lastSaveTime time.Time
	const saveDebounce = 500 * time.Millisecond // Don't save more than once per 500ms
	const checkInterval = 200 * time.Millisecond

	// Get initial position and save it
	if a.ctx != nil && a.configService != nil {
		lastX, lastY = runtime.WindowGetPosition(a.ctx)
		lastW, lastH = runtime.WindowGetSize(a.ctx)
		a.logger.Printf("[App] monitorWindowPosition: Initial position %dx%d at (%d,%d)", lastW, lastH, lastX, lastY)
		// Save initial position
		if lastW > 0 && lastH > 0 {
			if err := a.configService.SetWindowGeometry(lastX, lastY, lastW, lastH); err != nil {
				a.logger.Printf("[App] monitorWindowPosition: Failed to save initial position: %v", err)
			} else {
				a.logger.Printf("[App] monitorWindowPosition: Saved initial position")
			}
		}
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.logger.Printf("[App] monitorWindowPosition: Context cancelled, stopping monitor")
			return
		case <-ticker.C:
			if a.ctx == nil || a.configService == nil {
				continue
			}

			x, y := runtime.WindowGetPosition(a.ctx)
			w, h := runtime.WindowGetSize(a.ctx)

			// Check if position or size changed
			if x != lastX || y != lastY || w != lastW || h != lastH {
				// Position changed - check debounce
				if time.Since(lastSaveTime) >= saveDebounce && w > 0 && h > 0 {
					a.logger.Printf("[App] monitorWindowPosition: Window moved/resized to %dx%d at (%d,%d)", w, h, x, y)
					if err := a.configService.SetWindowGeometry(x, y, w, h); err != nil {
						a.logger.Printf("[App] monitorWindowPosition: Failed to save: %v", err)
					}
					lastSaveTime = time.Now()
				}
				lastX, lastY, lastW, lastH = x, y, w, h
			}
		}
	}
}

// startPrereqPolling - DISABLED: Prerequisites now only run once on startup and manually via RefreshNow()
// This function has been disabled to prevent automatic polling every 30 seconds.
// If periodic checks are needed in the future, this can be re-enabled.
/*
func (a *App) startPrereqPolling(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.logger.Printf("[App] startPrereqPolling: Context cancelled, stopping polling")
			return
		case <-ticker.C:
			report := a.prereqService.GetPrereqReport()
			runtime.EventsEmit(ctx, "PrereqReport", report)
		}
	}
}
*/

// Run starts the Wails application
func Run() error {
	appStartTime := time.Now()
	appTimestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logger := log.New(os.Stderr, "[GusSync] ", log.LstdFlags|log.Lshortfile)
	logger.Printf("[TIMING %s] [App] Run(): ENTRY - App Run() called, beginning initialization", appTimestamp)

	appInstance := NewApp()

	// Create a temporary context for service initialization
	// Services will be fully initialized in OnStartup
	ctx := context.Background()
	
	// Pre-initialize services for binding generation
	// Wails needs these to generate the bindings
	serviceInitStart := time.Now()
	jobManager := services.NewJobManager(ctx, logger)
	prereqService := services.NewPrereqService(ctx, logger)
	deviceService := services.NewDeviceService(ctx, logger)
	copyService := services.NewCopyService(ctx, logger, jobManager, deviceService)
	verifyService := services.NewVerifyService(ctx, logger, jobManager, deviceService)
	cleanupService := services.NewCleanupService(ctx, logger, jobManager, deviceService)
	logService := services.NewLogService(ctx, logger)
	systemService := services.NewSystemService(ctx, logger)
	serviceInitDuration := time.Since(serviceInitStart)
	logger.Printf("[TIMING %s] [App] Run(): Services pre-initialized (took %v)", time.Now().Format("2006-01-02 15:04:05.000"), serviceInitDuration)

	// Store services in app instance so they can be re-initialized in OnStartup
	appInstance.jobManager = jobManager
	appInstance.prereqService = prereqService
	appInstance.deviceService = deviceService
	appInstance.copyService = copyService
	appInstance.verifyService = verifyService
	appInstance.cleanupService = cleanupService
	appInstance.logService = logService
	appInstance.systemService = systemService

	wailsCallStart := time.Now()
	logger.Printf("[TIMING %s] [App] Run(): About to call wails.Run() - initialization took %v so far", time.Now().Format("2006-01-02 15:04:05.000"), time.Since(appStartTime))
	logger.Printf("[TIMING %s] [App] Run(): ⚠️  BLOCKING CALL ⚠️  - wails.Run() will block until frontend loads...", time.Now().Format("2006-01-02 15:04:05.000"))

	err := wails.Run(&options.App{
		Title:  "GusSync",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: nil, // Use default handler for embedded assets
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        appInstance.OnStartup,
		OnBeforeClose:    appInstance.OnBeforeClose,
		OnShutdown:       appInstance.OnShutdown,
		Bind: []interface{}{
			prereqService,
			deviceService,
			copyService,
			verifyService,
			cleanupService,
			logService,
			jobManager,
			systemService,
		},
	})
	
	wailsReturnDuration := time.Since(wailsCallStart)
	if err != nil {
		logger.Printf("[TIMING %s] [App] Run(): wails.Run() returned ERROR after %v: %v", time.Now().Format("2006-01-02 15:04:05.000"), wailsReturnDuration, err)
	} else {
		logger.Printf("[TIMING %s] [App] Run(): wails.Run() returned successfully after %v", time.Now().Format("2006-01-02 15:04:05.000"), wailsReturnDuration)
	}

	return err
}

