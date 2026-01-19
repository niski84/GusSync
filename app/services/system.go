package services

import (
	"context"
	"log"
	"os/exec"
	goruntime "runtime"
)

type SystemService struct {
	ctx    context.Context
	logger *log.Logger
}

func NewSystemService(ctx context.Context, logger *log.Logger) *SystemService {
	return &SystemService{
		ctx:    ctx,
		logger: logger,
	}
}

func (s *SystemService) SetContext(ctx context.Context) {
	s.ctx = ctx
}

func (s *SystemService) OpenPath(path string) error {
	s.logger.Printf("[SystemService] OpenPath: %s", path)
	var cmd *exec.Cmd

	switch goruntime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default: // linux and others
		cmd = exec.Command("xdg-open", path)
	}

	return cmd.Start()
}

