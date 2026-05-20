//go:build darwin

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultMacOSHelperPath = "macos-helper/.build/release/VoicePasteHelper"

type macOSPasteRequest struct {
	Text           string     `json:"text"`
	Chord          pasteChord `json:"chord"`
	DelayMS        int        `json:"delay_ms"`
	RestoreDelayMS int        `json:"restore_delay_ms"`
}

func newPasteAction(delay time.Duration, logger *slog.Logger) func(context.Context, string, pasteChord) error {
	helperPath := macOSHelperPath(logger)
	delayMS := int(delay / time.Millisecond)
	restoreDelayMS := int(pasteRestoreDelayFromEnv(logger) / time.Millisecond)

	return func(ctx context.Context, text string, chord pasteChord) error {
		request := macOSPasteRequest{
			Text:           text,
			Chord:          chord,
			DelayMS:        delayMS,
			RestoreDelayMS: restoreDelayMS,
		}

		payload, err := json.Marshal(request)
		if err != nil {
			return fmt.Errorf("marshal macOS paste request: %w", err)
		}

		ctx, cancel := context.WithTimeout(ctx, commandTimeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, helperPath)
		cmd.Stdin = bytes.NewReader(payload)
		output, err := cmd.CombinedOutput()
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("macOS helper timed out after %s", commandTimeout)
			}
			return commandError("macOS helper", output, err)
		}
		return nil
	}
}

func macOSHelperPath(logger *slog.Logger) string {
	configured := strings.TrimSpace(os.Getenv("VOICE_DAEMON_MACOS_HELPER"))
	if configured != "" {
		return configured
	}

	if _, err := os.Stat(defaultMacOSHelperPath); err == nil {
		return defaultMacOSHelperPath
	}

	executable, err := os.Executable()
	if err != nil {
		logger.Warn("could not inspect executable path for macOS helper fallback", "err", err)
		return "VoicePasteHelper"
	}

	nextToDaemon := filepath.Join(filepath.Dir(executable), "VoicePasteHelper")
	if _, err := os.Stat(nextToDaemon); err == nil {
		return nextToDaemon
	}

	return "VoicePasteHelper"
}
