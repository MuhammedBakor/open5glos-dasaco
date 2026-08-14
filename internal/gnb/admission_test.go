package gnb

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func performAdmissionRequest(
	t *testing.T,
	method string,
	body string,
) AdmissionStatus {
	t.Helper()

	request := httptest.NewRequest(
		method,
		"/admission",
		bytes.NewBufferString(body),
	)

	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	recorder := httptest.NewRecorder()

	admissionHandler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP 200, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var status AdmissionStatus

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&status,
	); err != nil {
		t.Fatalf(
			"failed to decode response: %v",
			err,
		)
	}

	return status
}

func TestOpenMode(t *testing.T) {
	status := performAdmissionRequest(
		t,
		http.MethodPut,
		`{"mode":"OPEN"}`,
	)

	if status.Mode != AdmissionModeOpen {
		t.Fatalf(
			"expected mode OPEN, got %s",
			status.Mode,
		)
	}

	if status.Enabled {
		t.Fatal(
			"OPEN mode must disable admission limiting",
		)
	}
}

func TestStrongProtectionMode(t *testing.T) {
	status := performAdmissionRequest(
		t,
		http.MethodPut,
		`{"mode":"STRONG_PROTECTION"}`,
	)

	if status.Mode != AdmissionModeStrong {
		t.Fatalf(
			"expected STRONG_PROTECTION, got %s",
			status.Mode,
		)
	}

	if !status.Enabled {
		t.Fatal(
			"STRONG_PROTECTION must be enabled",
		)
	}

	if status.RatePerSecond != 15 {
		t.Fatalf(
			"expected rate 15, got %.2f",
			status.RatePerSecond,
		)
	}

	if status.Burst != 5 {
		t.Fatalf(
			"expected burst 5, got %d",
			status.Burst,
		)
	}
}

func TestCustomMode(t *testing.T) {
	status := performAdmissionRequest(
		t,
		http.MethodPut,
		`{
			"mode":"CUSTOM",
			"rate_per_second":20,
			"burst":8
		}`,
	)

	if status.Mode != AdmissionModeCustom {
		t.Fatalf(
			"expected CUSTOM, got %s",
			status.Mode,
		)
	}

	if status.RatePerSecond != 20 {
		t.Fatalf(
			"expected rate 20, got %.2f",
			status.RatePerSecond,
		)
	}

	if status.Burst != 8 {
		t.Fatalf(
			"expected burst 8, got %d",
			status.Burst,
		)
	}
}

func TestOpenModeDoesNotBlock(t *testing.T) {
	dynamicAdmission.setPolicy(
		AdmissionModeOpen,
		false,
		0,
		1000,
	)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		100*time.Millisecond,
	)
	defer cancel()

	if err := WaitForRegistrationAdmission(ctx); err != nil {
		t.Fatalf(
			"OPEN mode returned an error: %v",
			err,
		)
	}
}

func TestHealthEndpoint(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodGet,
		"/healthz",
		nil,
	)

	recorder := httptest.NewRecorder()

	healthHandler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP 200, got %d",
			recorder.Code,
		)
	}

	if recorder.Body.String() != `{"status":"ok"}` {
		t.Fatalf(
			"unexpected health response: %s",
			recorder.Body.String(),
		)
	}
}
