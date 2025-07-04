package utils

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var logger zerolog.Logger

// LogLevel represents the available log levels
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
	LogLevelFatal LogLevel = "fatal"
)

// LoggerConfig holds configuration for the logger
type LoggerConfig struct {
	Level      LogLevel `yaml:"level"`
	Format     string   `yaml:"format"` // "json" or "console"
	TimeFormat string   `yaml:"time_format"`
}

// InitLogger initializes the global logger with the specified configuration
func InitLogger(config LoggerConfig) {
	// Set default values if not provided
	if config.Level == "" {
		config.Level = LogLevelInfo
	}
	if config.Format == "" {
		config.Format = "json"
	}
	if config.TimeFormat == "" {
		config.TimeFormat = time.RFC3339
	}

	// Set log level
	switch strings.ToLower(string(config.Level)) {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case "fatal":
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	// Configure time format
	zerolog.TimeFieldFormat = config.TimeFormat

	// Configure output format
	if strings.ToLower(config.Format) == "console" {
		// Human-readable console output
		logger = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: config.TimeFormat,
		}).With().Timestamp().Caller().Logger()
	} else {
		// JSON structured output
		logger = zerolog.New(os.Stdout).With().Timestamp().Caller().Logger()
	}

	// Set as global logger
	log.Logger = logger
}

// InitDefaultLogger initializes the logger with default settings
func InitDefaultLogger() {
	InitLogger(LoggerConfig{
		Level:      LogLevelInfo,
		Format:     "json",
		TimeFormat: time.RFC3339,
	})
}

// LogInfo logs an info level message
func LogInfo(message string, fields ...map[string]interface{}) {
	event := logger.Info()
	addFields(event, fields...)
	event.Msg(message)
}

// LogError logs an error level message
func LogError(message string, err error, fields ...map[string]interface{}) {
	event := logger.Error()
	if err != nil {
		event = event.Err(err)
	}
	addFields(event, fields...)
	event.Msg(message)
}

// LogDebug logs a debug level message
func LogDebug(message string, fields ...map[string]interface{}) {
	event := logger.Debug()
	addFields(event, fields...)
	event.Msg(message)
}

// LogWarn logs a warning level message
func LogWarn(message string, fields ...map[string]interface{}) {
	event := logger.Warn()
	addFields(event, fields...)
	event.Msg(message)
}

// LogFatal logs a fatal level message and exits
func LogFatal(message string, err error, fields ...map[string]interface{}) {
	event := logger.Fatal()
	if err != nil {
		event = event.Err(err)
	}
	addFields(event, fields...)
	event.Msg(message)
}

// addFields adds key-value pairs to a log event
func addFields(event *zerolog.Event, fields ...map[string]interface{}) {
	for _, fieldMap := range fields {
		for key, value := range fieldMap {
			event = event.Interface(key, value)
		}
	}
}

// GetLogger returns the configured logger instance
func GetLogger() zerolog.Logger {
	return logger
}

// WithFields creates a new logger with additional fields
func WithFields(fields map[string]interface{}) zerolog.Logger {
	ctx := logger.With()
	for key, value := range fields {
		ctx = ctx.Interface(key, value)
	}
	return ctx.Logger()
}

// SetLogLevel dynamically changes the log level
func SetLogLevel(level LogLevel) {
	switch strings.ToLower(string(level)) {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case "fatal":
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	}
}
