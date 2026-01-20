package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"GusSync/internal/core"
)

// handleSSE handles Server-Sent Events for real-time updates
// Clients connect to /api/events and receive job updates as they happen
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed")
		return
	}

	// Check if client supports SSE
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "sse_not_supported", "Streaming not supported")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a channel for this client
	clientChan := make(chan core.JobUpdateEvent, 100) // Buffer to prevent blocking
	s.addSSEClient(clientChan)
	defer s.removeSSEClient(clientChan)

	// Send initial connected event
	s.sendSSEEvent(w, "connected", map[string]interface{}{
		"message": "Connected to GusSync event stream",
	})
	flusher.Flush()

	// Send current job state if there's an active job
	if activeJob := s.jobManager.GetActiveJob(); activeJob != nil {
		s.sendSSEEvent(w, "job:snapshot", activeJob)
		flusher.Flush()
	}

	// Listen for events
	s.logger.Printf("[API] SSE client connected, waiting for events...")
	for {
		select {
		case <-r.Context().Done():
			s.logger.Printf("[API] SSE client disconnected (context done)")
			return
		case event, ok := <-clientChan:
			if !ok {
				s.logger.Printf("[API] SSE client channel closed")
				return
			}

			// Determine event type based on job state
			eventType := "job:update"
			switch event.State {
			case core.JobSucceeded:
				eventType = "job:completed"
			case core.JobFailed:
				eventType = "job:failed"
			case core.JobCanceled:
				eventType = "job:canceled"
			}

			// If there's a log line, emit a separate event
			if event.LogLine != "" {
				s.sendSSEEvent(w, "job:log", map[string]interface{}{
					"jobId":   event.JobID,
					"logLine": event.LogLine,
					"seq":     event.Seq,
				})
				flusher.Flush()
			}

			// Always emit the update event
			s.sendSSEEvent(w, eventType, event)
			flusher.Flush()
		}
	}
}

// sendSSEEvent writes an SSE event to the response writer
func (s *Server) sendSSEEvent(w http.ResponseWriter, eventType string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		s.logger.Printf("[API] SSE marshal error: %v", err)
		return
	}

	// SSE format: event: <type>\ndata: <json>\n\n
	fmt.Fprintf(w, "event: %s\n", eventType)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
}

