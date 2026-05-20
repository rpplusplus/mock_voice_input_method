package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultAddr = "0.0.0.0:47832"
	maxBodySize = 1 << 20
)

type server struct {
	token string
	paste func(context.Context, string, pasteChord) error
	log   *slog.Logger
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	addr := strings.TrimSpace(os.Getenv("VOICE_DAEMON_ADDR"))
	if addr == "" {
		addr = defaultAddr
	}

	token := strings.TrimSpace(os.Getenv("VOICE_DAEMON_TOKEN"))
	if token == "" {
		logger.Error("VOICE_DAEMON_TOKEN is required")
		os.Exit(1)
	}

	srv := &server{
		token: token,
		paste: newPasteAction(pasteDelayFromEnv(logger), logger),
		log:   logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", srv.handleHealth)
	mux.HandleFunc("POST /type", srv.handleType)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("mock voice input daemon listening", "addr", addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

func (s *server) handleType(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized\n", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodySize))
	if err != nil {
		http.Error(w, "request body too large\n", http.StatusRequestEntityTooLarge)
		return
	}
	defer r.Body.Close()

	text := string(body)
	if strings.TrimSpace(text) == "" {
		http.Error(w, "empty text\n", http.StatusBadRequest)
		return
	}

	if err := s.paste(r.Context(), text, pasteChordFromRequest(r)); err != nil {
		s.log.Error("paste failed", "err", err)
		http.Error(w, fmt.Sprintf("paste failed: %v\n", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *server) authorized(r *http.Request) bool {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix)) == s.token
}

func pasteChordFromRequest(r *http.Request) pasteChord {
	switch strings.TrimSpace(r.Header.Get("X-Voice-Paste-Chord")) {
	case string(chordCtrlShiftV):
		return chordCtrlShiftV
	default:
		return chordCtrlV
	}
}
