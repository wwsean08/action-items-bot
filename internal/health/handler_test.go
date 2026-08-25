package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeGatewayChecker struct {
	err error
}

func (f fakeGatewayChecker) GatewayHealthy() error {
	return f.err
}

type fakeDatabasePinger struct {
	err error
}

func (f fakeDatabasePinger) Ping(_ context.Context) error {
	return f.err
}

func doRequest(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandler_ReturnsOKWhenBothChecksPass(t *testing.T) {
	h := NewHandler(fakeGatewayChecker{}, fakeDatabasePinger{})

	rec := doRequest(t, h)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandler_ReturnsServiceUnavailableWhenGatewayUnhealthy(t *testing.T) {
	h := NewHandler(fakeGatewayChecker{err: errors.New("gateway down")}, fakeDatabasePinger{})

	rec := doRequest(t, h)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(rec.Body.String(), "gateway down") {
		t.Errorf("body = %q, want it to mention the gateway failure", rec.Body.String())
	}
}

func TestHandler_ReturnsServiceUnavailableWhenDatabaseUnhealthy(t *testing.T) {
	h := NewHandler(fakeGatewayChecker{}, fakeDatabasePinger{err: errors.New("db unreachable")})

	rec := doRequest(t, h)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(rec.Body.String(), "db unreachable") {
		t.Errorf("body = %q, want it to mention the database failure", rec.Body.String())
	}
}

func TestHandler_ReportsBothFailuresWhenBothUnhealthy(t *testing.T) {
	h := NewHandler(
		fakeGatewayChecker{err: errors.New("gateway down")},
		fakeDatabasePinger{err: errors.New("db unreachable")},
	)

	rec := doRequest(t, h)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "gateway down") || !strings.Contains(body, "db unreachable") {
		t.Errorf("body = %q, want it to mention both failures", body)
	}
}
