package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"service": "gussync-api",
	})
}

// handleJobs returns all jobs
func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed")
		return
	}

	jobs := s.jobManager.ListJobs()
	activeJob := s.jobManager.GetActiveJob()

	activeJobID := ""
	if activeJob != nil {
		activeJobID = activeJob.JobID
	}

	s.writeJSON(w, http.StatusOK, JobListResponse{
		Jobs:      jobs,
		ActiveJob: activeJobID,
	})
}

// handleActiveJob returns the currently active job
func (s *Server) handleActiveJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed")
		return
	}

	job := s.jobManager.GetActiveJob()
	if job == nil {
		s.writeJSON(w, http.StatusOK, nil)
		return
	}

	s.writeJSON(w, http.StatusOK, job)
}

// handleJob handles operations on a specific job: GET /api/jobs/{id} or DELETE /api/jobs/{id}
func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	// Parse job ID from path: /api/jobs/{id} or /api/jobs/{id}/cancel
	path := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_path", "Job ID required")
		return
	}

	jobID := parts[0]
	isCancel := len(parts) > 1 && parts[1] == "cancel"

	switch r.Method {
	case http.MethodGet:
		job, err := s.jobManager.GetJob(jobID)
		if err != nil {
			s.writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, job)

	case http.MethodDelete:
		if err := s.jobManager.CancelJob(jobID); err != nil {
			s.writeError(w, http.StatusBadRequest, "cancel_failed", err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]string{
			"message": fmt.Sprintf("Job %s cancellation requested", jobID),
		})

	case http.MethodPost:
		if isCancel {
			if err := s.jobManager.CancelJob(jobID); err != nil {
				s.writeError(w, http.StatusBadRequest, "cancel_failed", err.Error())
				return
			}
			s.writeJSON(w, http.StatusOK, map[string]string{
				"message": fmt.Sprintf("Job %s cancellation requested", jobID),
			})
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST /api/jobs/{id}/cancel to cancel")
		}

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET, DELETE, or POST to /cancel allowed")
	}
}

// handlePrereqs returns the prerequisites report
func (s *Server) handlePrereqs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed")
		return
	}

	if s.prereqProvider == nil {
		s.writeError(w, http.StatusNotImplemented, "not_implemented", "Prereq provider not configured")
		return
	}

	report := s.prereqProvider()
	s.writeJSON(w, http.StatusOK, report)
}

// handleDevices returns the connected devices
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed")
		return
	}

	if s.deviceProvider == nil {
		s.writeError(w, http.StatusNotImplemented, "not_implemented", "Device provider not configured")
		return
	}

	devices := s.deviceProvider()
	s.writeJSON(w, http.StatusOK, devices)
}

// handleConfig returns the current configuration
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed")
		return
	}

	if s.configProvider == nil {
		s.writeError(w, http.StatusNotImplemented, "not_implemented", "Config provider not configured")
		return
	}

	config := s.configProvider()
	s.writeJSON(w, http.StatusOK, config)
}

// handleStartCopy starts a new copy operation
func (s *Server) handleStartCopy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed")
		return
	}

	if s.startCopyFunc == nil {
		s.writeError(w, http.StatusNotImplemented, "not_implemented", "Start copy function not configured")
		return
	}

	var req StartCopyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Empty body is OK - will use defaults from config
		req = StartCopyRequest{}
	}

	jobID, err := s.startCopyFunc(r.Context(), req)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "start_failed", err.Error())
		return
	}

	s.writeJSON(w, http.StatusAccepted, map[string]string{
		"jobId":   jobID,
		"message": "Copy operation started",
	})
}

