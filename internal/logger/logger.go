package logger

import (
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.Logger
var atomicLevel zap.AtomicLevel

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
	Output     string   `yaml:"output"` // "stdout", "stderr", or file path
}

func InitLogger(config LoggerConfig) error {
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

	// Parse log level
	level, err := parseLogLevel(string(config.Level))
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}

	// Set atomic level
	atomicLevel = zap.NewAtomicLevelAt(level)

	// Configure encoder
	var encoderConfig zapcore.EncoderConfig
	if config.Format == "console" {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		encoderConfig = zap.NewProductionEncoderConfig()
		encoderConfig.TimeKey = "timestamp"
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	// Configure output
	var output zapcore.WriteSyncer
	switch config.Output {
	case "stdout":
		output = zapcore.AddSync(os.Stdout)
	case "stderr":
		output = zapcore.AddSync(os.Stderr)
	default:
		file, err := os.OpenFile(config.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		output = zapcore.AddSync(file)
	}

	// Create encoder
	var encoder zapcore.Encoder
	if config.Format == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// Create core
	core := zapcore.NewCore(encoder, output, atomicLevel)

	// Create logger
	logger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	return nil
}

// parseLogLevel converts string level to zapcore.Level
func parseLogLevel(level string) (zapcore.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	case "fatal":
		return zapcore.FatalLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("unknown level: %s", level)
	}
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
	if logger == nil {
		return
	}
	if len(fields) > 0 {
		logger.Info(message, mapToZapFields(fields...)...)
	} else {
		logger.Info(message)
	}
}

// LogError logs an error level message
func LogError(message string, err error, fields ...map[string]interface{}) {
	if logger == nil {
		return
	}
	zapFields := mapToZapFields(fields...)
	if err != nil {
		zapFields = append(zapFields, zap.Error(err))
	}
	logger.Error(message, zapFields...)
}

// LogDebug logs a debug level message
func LogDebug(message string, fields ...map[string]interface{}) {
	if logger == nil {
		return
	}
	logger.Debug(message, mapToZapFields(fields...)...)
}

// LogWarn logs a warning level message
func LogWarn(message string, fields ...map[string]interface{}) {
	if logger == nil {
		return
	}
	logger.Warn(message, mapToZapFields(fields...)...)
}

// LogFatal logs a fatal level message and exits
func LogFatal(message string, err error, fields ...map[string]interface{}) {
	if logger == nil {
		return
	}
	zapFields := mapToZapFields(fields...)
	if err != nil {
		zapFields = append(zapFields, zap.Error(err))
	}
	logger.Fatal(message, zapFields...)
}

// mapToZapFields converts map fields to zap.Field slice
func mapToZapFields(fields ...map[string]interface{}) []zap.Field {
	var zapFields []zap.Field
	for _, fieldMap := range fields {
		for key, value := range fieldMap {
			zapFields = append(zapFields, zap.Any(key, value))
		}
	}
	return zapFields
}

// GetLogger returns the configured logger instance
func GetLogger() *zap.Logger {
	return logger
}

// WithFields creates a new logger with additional fields
// SetLogLevel dynamically changes the log level
func SetLogLevel(level LogLevel) {
	newLevel, err := parseLogLevel(string(level))
	if err == nil {
		atomicLevel.SetLevel(newLevel)
	}
}
