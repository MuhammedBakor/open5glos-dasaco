package gnb

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

const (
	AdmissionModeOpen     = "OPEN"
	AdmissionModeMild     = "MILD_PROTECTION"
	AdmissionModeStrong   = "STRONG_PROTECTION"
	AdmissionModeRecovery = "RECOVERY"
	AdmissionModeCustom   = "CUSTOM"
)

type AdmissionStatus struct {
	Mode          string  `json:"mode"`
	Enabled       bool    `json:"enabled"`
	RatePerSecond float64 `json:"rate_per_second"`
	Burst         int     `json:"burst"`
	Admitted      uint64  `json:"admitted"`
	Waited        uint64  `json:"waited"`
	TotalWaitMs   uint64  `json:"total_wait_ms"`
	UpdatedAt     string  `json:"updated_at"`
}

type AdmissionUpdate struct {
	Mode          string   `json:"mode"`
	Enabled       *bool    `json:"enabled,omitempty"`
	RatePerSecond *float64 `json:"rate_per_second,omitempty"`
	Burst         *int     `json:"burst,omitempty"`
}

type admissionManager struct {
	mutex     sync.RWMutex
	limiter   *rate.Limiter
	mode      string
	enabled   bool
	rateValue float64
	burst     int
	updatedAt time.Time

	admitted    atomic.Uint64
	waited      atomic.Uint64
	totalWaitMs atomic.Uint64
}

var dynamicAdmission = newAdmissionManager()

func newAdmissionManager() *admissionManager {
	manager := &admissionManager{}
	manager.setPolicy(AdmissionModeOpen, false, 0, 1000)
	return manager
}

func (manager *admissionManager) setPolicy(
	mode string,
	enabled bool,
	rateValue float64,
	burst int,
) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	manager.mode = mode
	manager.enabled = enabled
	manager.rateValue = rateValue
	manager.burst = burst
	manager.updatedAt = time.Now().UTC()

	if enabled {
		manager.limiter = rate.NewLimiter(
			rate.Limit(rateValue),
			burst,
		)
	} else {
		manager.limiter = rate.NewLimiter(rate.Inf, burst)
	}

	log.Printf(
		"[DA-SACO][ADMISSION] mode=%s enabled=%t rate=%.2f burst=%d",
		mode,
		enabled,
		rateValue,
		burst,
	)
}

func (manager *admissionManager) wait(ctx context.Context) error {
	manager.mutex.RLock()
	enabled := manager.enabled
	limiter := manager.limiter
	manager.mutex.RUnlock()

	if !enabled {
		manager.admitted.Add(1)
		return nil
	}

	start := time.Now()
	err := limiter.Wait(ctx)
	waitMs := uint64(time.Since(start).Milliseconds())

	if err != nil {
		return err
	}

	manager.admitted.Add(1)

	if waitMs > 0 {
		manager.waited.Add(1)
		manager.totalWaitMs.Add(waitMs)
	}

	return nil
}

func (manager *admissionManager) status() AdmissionStatus {
	manager.mutex.RLock()
	defer manager.mutex.RUnlock()

	return AdmissionStatus{
		Mode:          manager.mode,
		Enabled:       manager.enabled,
		RatePerSecond: manager.rateValue,
		Burst:         manager.burst,
		Admitted:      manager.admitted.Load(),
		Waited:        manager.waited.Load(),
		TotalWaitMs:   manager.totalWaitMs.Load(),
		UpdatedAt:     manager.updatedAt.Format(time.RFC3339Nano),
	}
}

func (manager *admissionManager) update(
	update AdmissionUpdate,
) error {
	switch update.Mode {
	case AdmissionModeOpen:
		manager.setPolicy(
			AdmissionModeOpen,
			false,
			0,
			1000,
		)

	case AdmissionModeMild:
		manager.setPolicy(
			AdmissionModeMild,
			true,
			25,
			10,
		)

	case AdmissionModeStrong:
		manager.setPolicy(
			AdmissionModeStrong,
			true,
			15,
			5,
		)

	case AdmissionModeRecovery:
		manager.setPolicy(
			AdmissionModeRecovery,
			true,
			35,
			15,
		)

	case AdmissionModeCustom:
		if update.RatePerSecond == nil {
			return fmt.Errorf(
				"CUSTOM mode requires rate_per_second",
			)
		}

		if update.Burst == nil {
			return fmt.Errorf(
				"CUSTOM mode requires burst",
			)
		}

		if *update.RatePerSecond <= 0 {
			return fmt.Errorf(
				"rate_per_second must be positive",
			)
		}

		if *update.Burst <= 0 {
			return fmt.Errorf(
				"burst must be positive",
			)
		}

		enabled := true
		if update.Enabled != nil {
			enabled = *update.Enabled
		}

		manager.setPolicy(
			AdmissionModeCustom,
			enabled,
			*update.RatePerSecond,
			*update.Burst,
		)

	default:
		return fmt.Errorf(
			"unsupported admission mode: %s",
			update.Mode,
		)
	}

	return nil
}

func WaitForRegistrationAdmission(
	ctx context.Context,
) error {
	return dynamicAdmission.wait(ctx)
}

func admissionHandler(
	writer http.ResponseWriter,
	request *http.Request,
) {
	writer.Header().Set(
		"Content-Type",
		"application/json",
	)

	switch request.Method {
	case http.MethodGet:
		writer.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(writer).Encode(
			dynamicAdmission.status(),
		)

	case http.MethodPut, http.MethodPost:
		var update AdmissionUpdate

		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&update); err != nil {
			http.Error(
				writer,
				fmt.Sprintf("invalid request: %v", err),
				http.StatusBadRequest,
			)
			return
		}

		if err := dynamicAdmission.update(update); err != nil {
			http.Error(
				writer,
				err.Error(),
				http.StatusBadRequest,
			)
			return
		}

		writer.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(writer).Encode(
			dynamicAdmission.status(),
		)

	default:
		writer.Header().Set(
			"Allow",
			"GET, PUT, POST",
		)
		http.Error(
			writer,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
	}
}

func healthHandler(
	writer http.ResponseWriter,
	request *http.Request,
) {
	writer.Header().Set(
		"Content-Type",
		"application/json",
	)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(
		[]byte(`{"status":"ok"}`),
	)
}

func startAdmissionServer() {
	address := os.Getenv("ADMISSION_CONTROL_ADDRESS")
	if address == "" {
		address = ":9091"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/admission", admissionHandler)

	server := &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	go func() {
		log.Printf(
			"[DA-SACO][ADMISSION] API listening on %s",
			address,
		)

		err := server.ListenAndServe()

		if err != nil && err != http.ErrServerClosed {
			log.Printf(
				"[DA-SACO][ADMISSION][ERROR] %v",
				err,
			)
		}
	}()
}

func init() {
	startAdmissionServer()
}
