package services

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// ConfigService manages application configuration
type ConfigService struct {
	configPath string
	logger     *log.Logger
	config     *Config
}

// Config represents the application configuration
type Config struct {
	DestinationPath string `json:"destinationPath"`
	SourcePath      string `json:"sourcePath"`
	LastLogPath     string `json:"lastLogPath"`
	LogDir          string `json:"logDir"`
}

// NewConfigService creates a new ConfigService
func NewConfigService(logger *log.Logger) (*ConfigService, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".gussync")
	configPath := filepath.Join(configDir, "config.json")
	logDir := filepath.Join(configDir, "logs")

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	service := &ConfigService{
		configPath: configPath,
		logger:     logger,
		config: &Config{
			LogDir: logDir,
		},
	}

	// Load existing config if it exists
	if err := service.Load(); err != nil {
		logger.Printf("[ConfigService] Failed to load config: %v", err)
		// Continue with default config
	}

	return service, nil
}

// Load loads the configuration from disk
func (s *ConfigService) Load() error {
	s.logger.Printf("[ConfigService] Load: Loading config from %s", s.configPath)

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.logger.Printf("[ConfigService] Load: Config file does not exist, using defaults")
			return nil
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Ensure LogDir is set
	if config.LogDir == "" {
		homeDir, _ := os.UserHomeDir()
		config.LogDir = filepath.Join(homeDir, ".gussync", "logs")
	}

	s.config = &config
	s.logger.Printf("[ConfigService] Load: Config loaded: dest=%s, logDir=%s", config.DestinationPath, config.LogDir)
	return nil
}

// Save saves the configuration to disk
func (s *ConfigService) Save() error {
	s.logger.Printf("[ConfigService] Save: Saving config to %s", s.configPath)

	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	s.logger.Printf("[ConfigService] Save: Config saved successfully")
	return nil
}

// GetConfig returns the current configuration
func (s *ConfigService) GetConfig() Config {
	if s.config == nil {
		return Config{}
	}
	return *s.config
}

// SetDestinationPath sets the destination path and saves the config
func (s *ConfigService) SetDestinationPath(path string) error {
	s.logger.Printf("[ConfigService] SetDestinationPath: path=%s", path)

	if s.config == nil {
		s.config = &Config{}
	}

	s.config.DestinationPath = path
	return s.Save()
}

// SetLastLogPath sets the last log path and saves the config
func (s *ConfigService) SetLastLogPath(path string) error {
	s.logger.Printf("[ConfigService] SetLastLogPath: path=%s", path)

	if s.config == nil {
		s.config = &Config{}
	}

	s.config.LastLogPath = path
	return s.Save()
}

