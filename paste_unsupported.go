//go:build !linux && !darwin

package main

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"time"
)

func newPasteAction(_ time.Duration, _ *slog.Logger) func(context.Context, string, pasteChord) error {
	return func(context.Context, string, pasteChord) error {
		return fmt.Errorf("paste action is not implemented on %s yet", runtime.GOOS)
	}
}
