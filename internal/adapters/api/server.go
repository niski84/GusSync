package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"GusSync/internal/core"
)

// Server is the HTTP API server for GusSync
type Server struct {
	port       int
	logger     *log.Logger
	jobManager *core.JobManager
	server     *http.Server
	mux        *http.ServeMux

	// SSE clients
	sseClients   map[chan core.JobUpdateEvent]struct{}
	sseClientsMu sync.Mutex

	// Service providers (set via options)
	prereqProvider  func() interface{}
	deviceProvider  func() interface{}
	configProvider  func() interface{}
	startCopyFunc   func(ctx context.Context, req StartCopyRequest) (string, error)
}

// ServerOption configures the Server
type ServerOption func(*Server)

// WithPrereqProvider sets the function to get prerequisite status
func WithPrereqProvider(fn func() interface{}) ServerOption {
	return func(s *Server) {
		s.prereqProvider = fn
	}
}

// WithDeviceProvider sets the function to get device status
func WithDeviceProvider(fn func() interface{}) ServerOption {
	return func(s *Server) {
		s.deviceProvider = fn
	}
}

// WithConfigProvider sets the function to get configuration
func WithConfigProvider(fn func() interface{}) ServerOption {
	return func(s *Server) {
		s.configProvider = fn
	}
}

// WithStartCopyFunc sets the function to start a copy operation
func WithStartCopyFunc(fn func(ctx context.Context, req StartCopyRequest) (string, error)) ServerOption {
	return func(s *Server) {
		s.startCopyFunc = fn
	}
}

// NewServer creates a new API server
func NewServer(port int, logger *log.Logger, jobManager *core.JobManager, opts ...ServerOption) *Server {
	s := &Server{
		port:       port,
		logger:     logger,
		jobManager: jobManager,
		sseClients: make(map[chan core.JobUpdateEvent]struct{}),
	}

	for _, opt := range opts {
		opt(s)
	}

	s.setupRoutes()
	return s
}

// setupRoutes configures the HTTP routes
func (s *Server) setupRoutes() {
	s.mux = http.NewServeMux()

	// Health check
	s.mux.HandleFunc("/api/health", s.handleHealth)

	// Jobs API
	s.mux.HandleFunc("/api/jobs", s.handleJobs)
	s.mux.HandleFunc("/api/jobs/active", s.handleActiveJob)
	s.mux.HandleFunc("/api/jobs/", s.handleJob) // handles /api/jobs/{id} and /api/jobs/{id}/cancel

	// SSE events
	s.mux.HandleFunc("/api/events", s.handleSSE)

	// Prerequisites
	s.mux.HandleFunc("/api/prereqs", s.handlePrereqs)

	// Devices
	s.mux.HandleFunc("/api/devices", s.handleDevices)

	// Config
	s.mux.HandleFunc("/api/config", s.handleConfig)

	// Copy operations
	s.mux.HandleFunc("/api/copy/start", s.handleStartCopy)
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: s.corsMiddleware(s.loggingMiddleware(s.mux)),
	}

	s.logger.Printf("[API] Starting HTTP server on port %d", s.port)
	return s.server.ListenAndServe()
}

// StartBackground starts the server in a goroutine
func (s *Server) StartBackground(ctx context.Context) {
	go func() {
		if err := s.Start(); err != nil && err != http.ErrServerClosed {
			s.logger.Printf("[API] Server error: %v", err)
		}
	}()

	// Handle shutdown
	go func() {
		<-ctx.Done()
		s.logger.Printf("[API] Shutting down HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.logger.Printf("[API] Shutdown error: %v", err)
		}
	}()
}

// loggingMiddleware logs all requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Printf("[API] %s %s (took %v)", r.Method, r.URL.Path, time.Since(start))
	})
}

// corsMiddleware adds CORS headers for cross-origin requests
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// EmitJobUpdate implements core.JobEventEmitter to broadcast events to SSE clients
func (s *Server) EmitJobUpdate(event core.JobUpdateEvent) {
	s.sseClientsMu.Lock()
	defer s.sseClientsMu.Unlock()

	for clientChan := range s.sseClients {
		select {
		case clientChan <- event:
		default:
			// Client is slow, skip this event
			s.logger.Printf("[API] SSE client slow, skipping event")
		}
	}
}

// addSSEClient registers a new SSE client
func (s *Server) addSSEClient(ch chan core.JobUpdateEvent) {
	s.sseClientsMu.Lock()
	defer s.sseClientsMu.Unlock()
	s.sseClients[ch] = struct{}{}
	s.logger.Printf("[API] SSE client connected (total: %d)", len(s.sseClients))
}

// removeSSEClient unregisters an SSE client
func (s *Server) removeSSEClient(ch chan core.JobUpdateEvent) {
	s.sseClientsMu.Lock()
	defer s.sseClientsMu.Unlock()
	delete(s.sseClients, ch)
	close(ch)
	s.logger.Printf("[API] SSE client disconnected (total: %d)", len(s.sseClients))
}

// Helper functions for responses

func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    data,
	})
}

func (s *Server) writeError(w http.ResponseWriter, status int, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
		},
	})
}

