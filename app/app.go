package app

import (
	"context"
	"embed"
	"log"
	"os"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"GusSync/app/services"
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
	startTime := time.Now()
	a.ctx = ctx
	logger := log.New(os.Stderr, "[GusSync] ", log.LstdFlags|log.Lshortfile)
	
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logger.Printf("[TIMING %s] [App] OnStartup: ENTRY - OnStartup called by Wails (frontend should be loaded now)", timestamp)

	// Initialize config service first
	configStart := time.Now()
	configService, err := services.NewConfigService(logger)
	configDuration := time.Since(configStart)
	if err != nil {
		logger.Printf("[TIMING %s] [App] OnStartup: Config service initialization FAILED (took %v): %v", time.Now().Format("2006-01-02 15:04:05.000"), configDuration, err)
		// Continue without config service
	} else {
		logger.Printf("[TIMING %s] [App] OnStartup: Config service initialized (took %v)", time.Now().Format("2006-01-02 15:04:05.000"), configDuration)
	}

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

	totalServiceDuration := time.Since(startTime)
	logger.Printf("[TIMING %s] [App] OnStartup: All services updated (took %v total)", time.Now().Format("2006-01-02 15:04:05.000"), totalServiceDuration)

	// Run prerequisite checks immediately and cache results
	logger.Printf("[TIMING %s] [App] OnStartup: Running prerequisite checks...", time.Now().Format("2006-01-02 15:04:05.000"))
	go func() {
		checkStart := time.Now()
		report := a.prereqService.GetPrereqReport()
		checkDuration := time.Since(checkStart)
		logger.Printf("[TIMING %s] [App] OnStartup: Prerequisite checks completed (took %v) - report cached", time.Now().Format("2006-01-02 15:04:05.000"), checkDuration)
		logger.Printf("[App] OnStartup: Overall status: %s", report.OverallStatus)
	}()

	totalDuration := time.Since(startTime)
	logger.Printf("[TIMING %s] [App] OnStartup: EXIT - Total startup time: %v", time.Now().Format("2006-01-02 15:04:05.000"), totalDuration)
}

// OnShutdown is called when the app is shutting down
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
	copyService := services.NewCopyService(ctx, logger, jobManager)
	verifyService := services.NewVerifyService(ctx, logger, jobManager)
	cleanupService := services.NewCleanupService(ctx, logger, jobManager)
	logService := services.NewLogService(ctx, logger)
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
		OnShutdown:       appInstance.OnShutdown,
		Bind: []interface{}{
			prereqService,
			deviceService,
			copyService,
			verifyService,
			cleanupService,
			logService,
			jobManager,
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

