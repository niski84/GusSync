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
	"github.com/wailsapp/wails/v2/pkg/runtime"

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

// OnStartup is called when the app starts
func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx
	logger := log.New(os.Stderr, "[GusSync] ", log.LstdFlags|log.Lshortfile)

	// Initialize config service first
	configService, err := services.NewConfigService(logger)
	if err != nil {
		logger.Printf("[App] OnStartup: Failed to create config service: %v", err)
		// Continue without config service
	}

	// Initialize services
	a.jobManager = services.NewJobManager(ctx, logger)
	a.prereqService = services.NewPrereqService(ctx, logger)
	a.deviceService = services.NewDeviceService(ctx, logger)
	
	// Update existing copy service context (created in Run() for bindings)
	// The bound service instance needs to have its context updated
	a.copyService.SetContext(ctx)
	if configService != nil {
		a.copyService.SetConfig(configService)
	}
	
	a.verifyService = services.NewVerifyService(ctx, logger, a.jobManager)
	a.cleanupService = services.NewCleanupService(ctx, logger, a.jobManager)
	a.logService = services.NewLogService(ctx, logger)

	logger.Printf("[App] OnStartup: Services initialized")

	// Start prerequisite polling loop
	go a.startPrereqPolling(ctx)

	// Emit initial prerequisite report
	report := a.prereqService.GetPrereqReport()
	runtime.EventsEmit(ctx, "PrereqReport", report)

	logger.Printf("[App] OnStartup: Initial prerequisite report emitted")
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

// startPrereqPolling runs prerequisite checks periodically
func (a *App) startPrereqPolling(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Run immediately on startup
	report := a.prereqService.GetPrereqReport()
	runtime.EventsEmit(ctx, "PrereqReport", report)

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

// Run starts the Wails application
func Run() error {
	appInstance := NewApp()

	// Create a temporary context for service initialization
	// Services will be fully initialized in OnStartup
	ctx := context.Background()
	logger := log.New(os.Stderr, "[GusSync] ", log.LstdFlags|log.Lshortfile)
	
	// Pre-initialize services for binding generation
	// Wails needs these to generate the bindings
	jobManager := services.NewJobManager(ctx, logger)
	prereqService := services.NewPrereqService(ctx, logger)
	deviceService := services.NewDeviceService(ctx, logger)
	copyService := services.NewCopyService(ctx, logger, jobManager)
	verifyService := services.NewVerifyService(ctx, logger, jobManager)
	cleanupService := services.NewCleanupService(ctx, logger, jobManager)
	logService := services.NewLogService(ctx, logger)

	// Store services in app instance so they can be re-initialized in OnStartup
	appInstance.jobManager = jobManager
	appInstance.prereqService = prereqService
	appInstance.deviceService = deviceService
	appInstance.copyService = copyService
	appInstance.verifyService = verifyService
	appInstance.cleanupService = cleanupService
	appInstance.logService = logService

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

	return err
}

