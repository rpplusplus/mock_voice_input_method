//go:build linux

package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const keyDelayMS = "20"

func newPasteAction(delay time.Duration, _ *slog.Logger) func(context.Context, string, pasteChord) error {
	return func(ctx context.Context, text string, chord pasteChord) error {
		if err := runWithStdin(ctx, text, "wl-copy", "--type", "text/plain;charset=utf-8", "--sensitive"); err != nil {
			return fmt.Errorf("wl-copy: %w", err)
		}

		if err := sleepContext(ctx, delay); err != nil {
			return err
		}

		args := []string{"key", "--key-delay", keyDelayMS}
		args = append(args, linuxKeySequence(chord)...)

		if err := run(ctx, "ydotool", args...); err != nil {
			return fmt.Errorf("ydotool paste: %w", err)
		}

		return nil
	}
}

func linuxKeySequence(chord pasteChord) []string {
	if chord == chordCtrlShiftV {
		return []string{"29:1", "42:1", "47:1", "47:0", "42:0", "29:0"}
	}
	return []string{"29:1", "47:1", "47:0", "29:0"}
}
