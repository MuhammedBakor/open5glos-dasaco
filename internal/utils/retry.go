package utils

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// RetryConfig contains retry configuration
type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
	}
}

// RetryFunc is a function that can be retried
type RetryFunc func() error

// Retry executes a function with retry logic
func Retry(ctx context.Context, cfg RetryConfig, fn RetryFunc, logger *zap.Logger) error {
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		if err := fn(); err != nil {
			lastErr = err
			if attempt < cfg.MaxRetries {
				logger.Warn("Retry attempt failed",
					zap.Int("attempt", attempt+1),
					zap.Int("max_retries", cfg.MaxRetries),
					zap.Duration("delay", delay),
					zap.Error(err),
				)

				// Calculate next delay
				delay = time.Duration(float64(delay) * cfg.Multiplier)
				if delay > cfg.MaxDelay {
					delay = cfg.MaxDelay
				}
				continue
			}
		} else {
			// Success
			if attempt > 0 {
				logger.Info("Retry succeeded",
					zap.Int("attempts", attempt+1),
				)
			}
			return nil
		}
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}
