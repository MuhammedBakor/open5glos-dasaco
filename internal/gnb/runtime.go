package gnb

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

type RuntimeStatus struct {
	ActiveGnBConnections int64  `json:"active_gnb_connections"`
	Draining             bool   `json:"draining"`
	UpdatedAt            string `json:"updated_at"`
}

type DrainingUpdate struct {
	Draining *bool `json:"draining"`
}

var activeGnBConnections atomic.Int64
var drainingState atomic.Bool
var runtimeUpdatedAt atomic.Value

func init() {
	runtimeUpdatedAt.Store(time.Now().UTC())
}

func markRuntimeUpdated() {
	runtimeUpdatedAt.Store(time.Now().UTC())
}

func incrementActiveGnBConnections() {
	activeGnBConnections.Add(1)
	markRuntimeUpdated()
}

func decrementActiveGnBConnections() {
	for {
		current := activeGnBConnections.Load()

		if current <= 0 {
			activeGnBConnections.Store(0)
			return
		}

		if activeGnBConnections.CompareAndSwap(
			current,
			current-1,
		) {
			markRuntimeUpdated()
			return
		}
	}
}

func ActiveGnBConnectionCount() int64 {
	return activeGnBConnections.Load()
}

func IsDraining() bool {
	return drainingState.Load()
}

func SetDraining(value bool) {
	drainingState.Store(value)
	markRuntimeUpdated()
}

func RuntimeSnapshot() RuntimeStatus {
	updatedAt, _ := runtimeUpdatedAt.Load().(time.Time)

	return RuntimeStatus{
		ActiveGnBConnections: ActiveGnBConnectionCount(),
		Draining:             IsDraining(),
		UpdatedAt:            updatedAt.Format(time.RFC3339Nano),
	}
}

func runtimeHandler(
	writer http.ResponseWriter,
	request *http.Request,
) {
	writer.Header().Set(
		"Content-Type",
		"application/json",
	)

	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", "GET")
		http.Error(
			writer,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(
		RuntimeSnapshot(),
	)
}

func drainingHandler(
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
			RuntimeSnapshot(),
		)

	case http.MethodPut, http.MethodPost:
		var update DrainingUpdate

		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&update); err != nil {
			http.Error(
				writer,
				"invalid draining request",
				http.StatusBadRequest,
			)
			return
		}

		if update.Draining == nil {
			http.Error(
				writer,
				"draining is required",
				http.StatusBadRequest,
			)
			return
		}

		SetDraining(*update.Draining)

		writer.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(writer).Encode(
			RuntimeSnapshot(),
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
