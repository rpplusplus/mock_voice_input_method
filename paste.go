package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	commandTimeout      = 3 * time.Second
	defaultDelay        = 120 * time.Millisecond
	defaultRestoreDelay = 250 * time.Millisecond
)

type pasteChord string

const (
	chordCtrlV      pasteChord = "ctrl_v"
	chordCtrlShiftV pasteChord = "ctrl_shift_v"
)

func pasteDelayFromEnv(logger *slog.Logger) time.Duration {
	return durationFromEnv(logger, "VOICE_DAEMON_PASTE_DELAY_MS", defaultDelay)
}

func pasteRestoreDelayFromEnv(logger *slog.Logger) time.Duration {
	return durationFromEnv(logger, "VOICE_DAEMON_RESTORE_DELAY_MS", defaultRestoreDelay)
}

func durationFromEnv(logger *slog.Logger, name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}

	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 0 {
		logger.Warn("invalid duration env var, using default", "name", name, "value", raw, "default", fallback)
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
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
