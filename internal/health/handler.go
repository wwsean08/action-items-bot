package health

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type GatewayChecker interface {
	GatewayHealthy() error
}

type DatabasePinger interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	gateway  GatewayChecker
	database DatabasePinger
}

func NewHandler(gateway GatewayChecker, database DatabasePinger) *Handler {
	return &Handler{gateway: gateway, database: database}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var failures []string

	if err := h.gateway.GatewayHealthy(); err != nil {
		failures = append(failures, fmt.Sprintf("discord gateway: %v", err))
	}
	if err := h.database.Ping(r.Context()); err != nil {
		failures = append(failures, fmt.Sprintf("database: %v", err))
	}

	if len(failures) > 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, strings.Join(failures, "\n"))
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
}
