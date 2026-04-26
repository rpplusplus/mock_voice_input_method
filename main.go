package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAddr    = "0.0.0.0:47832"
	maxBodySize    = 1 << 20
	commandTimeout = 3 * time.Second
	defaultDelay   = 120 * time.Millisecond
	keyDelayMS     = "20"
)

type pasteFunc func(context.Context, string) error

type server struct {
	token string
	paste func(context.Context, string, pasteChord) error
	log   *slog.Logger
}

type pasteChord string

const (
	chordCtrlV      pasteChord = "ctrl_v"
	chordCtrlShiftV pasteChord = "ctrl_shift_v"
)

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
		paste: pasteWithTools(pasteDelayFromEnv(logger)),
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

func pasteDelayFromEnv(logger *slog.Logger) time.Duration {
	raw := strings.TrimSpace(os.Getenv("VOICE_DAEMON_PASTE_DELAY_MS"))
	if raw == "" {
		return defaultDelay
	}

	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 0 {
		logger.Warn("invalid VOICE_DAEMON_PASTE_DELAY_MS, using default", "value", raw, "default", defaultDelay)
		return defaultDelay
	}
	return time.Duration(ms) * time.Millisecond
}

func pasteChordFromRequest(r *http.Request) pasteChord {
	switch strings.TrimSpace(r.Header.Get("X-Voice-Paste-Chord")) {
	case string(chordCtrlShiftV):
		return chordCtrlShiftV
	default:
		return chordCtrlV
	}
}

func pasteWithTools(delay time.Duration) func(context.Context, string, pasteChord) error {
	return func(ctx context.Context, text string, chord pasteChord) error {
		if err := runWithStdin(ctx, text, "wl-copy", "--type", "text/plain;charset=utf-8", "--sensitive"); err != nil {
			return fmt.Errorf("wl-copy: %w", err)
		}

		if err := sleepContext(ctx, delay); err != nil {
			return err
		}

		args := []string{"key", "--key-delay", keyDelayMS}
		args = append(args, keySequence(chord)...)

		if err := run(ctx, "ydotool", args...); err != nil {
			return fmt.Errorf("ydotool ctrl+v: %w", err)
		}

		return nil
	}
}

func keySequence(chord pasteChord) []string {
	if chord == chordCtrlShiftV {
		return []string{"29:1", "42:1", "47:1", "47:0", "42:0", "29:0"}
	}
	return []string{"29:1", "47:1", "47:0", "29:0"}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func runWithStdin(ctx context.Context, stdin, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("timed out after %s", commandTimeout)
		}
		return err
	}
	return nil
}

func run(ctx context.Context, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("timed out after %s", commandTimeout)
		}
		return commandError(name, output, err)
	}
	return nil
}

func commandError(name string, output []byte, err error) error {
	msg := strings.TrimSpace(string(output))
	if msg == "" {
		return err
	}
	return fmt.Errorf("%w: %s: %s", err, name, msg)
}
