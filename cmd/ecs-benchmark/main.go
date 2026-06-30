package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"ecs-benchmark/internal/load"
)

type response struct {
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func main() {
	addr := envString("ADDR", ":8080")
	workers := envInt("WORKERS", 0)
	maxMemoryMB := envInt("MAX_MEMORY_MB", 4096)

	controller := load.NewController(workers, maxMemoryMB)
	defer controller.Stop()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /load", handleGetLoad(controller))
	mux.HandleFunc("POST /load", handlePostLoad(controller))
	mux.HandleFunc("POST /reset", handleReset(controller))

	server := &http.Server{
		Addr:              addr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("starting ecs-benchmark", "addr", addr, "workers", controller.Snapshot().Workers, "max_memory_mb", maxMemoryMB)
		errCh <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-stop:
		slog.Info("shutting down", "signal", sig.String())
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}

	if err := server.Close(); err != nil {
		slog.Error("server close failed", "error", err)
		os.Exit(1)
	}
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, response{Data: map[string]string{"status": "ok"}})
}

func handleGetLoad(controller *load.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, response{Data: controller.Snapshot()})
	}
}

func handlePostLoad(controller *load.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req load.Config
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: "invalid json body: " + err.Error()})
			return
		}

		snapshot, err := controller.Apply(req)
		if err != nil {
			status := http.StatusBadRequest
			writeJSON(w, status, response{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, response{Data: snapshot})
	}
}

func handleReset(controller *load.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		controller.Stop()
		writeJSON(w, http.StatusOK, response{Data: controller.Snapshot()})
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

func writeJSON(w http.ResponseWriter, status int, body response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write response failed", "error", err)
	}
}

func envString(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		slog.Warn("invalid integer env value, using fallback", "key", key, "value", value, "fallback", fallback)
		return fallback
	}
	return parsed
}
