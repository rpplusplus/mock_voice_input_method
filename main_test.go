package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleTypePastesAuthorizedText(t *testing.T) {
	var pasted string
	srv := &server{
		token: "test-token",
		paste: func(_ context.Context, text string, _ pasteChord) error {
			pasted = text
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/type", strings.NewReader("你好，世界"))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()

	srv.handleType(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if pasted != "你好，世界" {
		t.Fatalf("pasted = %q", pasted)
	}
}

func TestHandleTypeRejectsBadToken(t *testing.T) {
	srv := &server{
		token: "test-token",
		paste: func(context.Context, string, pasteChord) error {
			t.Fatal("paste should not be called")
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/type", strings.NewReader("hello"))
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()

	srv.handleType(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleTypeRejectsEmptyText(t *testing.T) {
	srv := &server{
		token: "test-token",
		paste: func(context.Context, string, pasteChord) error {
			t.Fatal("paste should not be called")
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/type", strings.NewReader(" \n\t "))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()

	srv.handleType(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleTypeUsesRequestedPasteChord(t *testing.T) {
	var chord pasteChord
	srv := &server{
		token: "test-token",
		paste: func(_ context.Context, _ string, requested pasteChord) error {
			chord = requested
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/type", strings.NewReader("hello"))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("X-Voice-Paste-Chord", string(chordCtrlShiftV))
	rec := httptest.NewRecorder()

	srv.handleType(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if chord != chordCtrlShiftV {
		t.Fatalf("chord = %q, want %q", chord, chordCtrlShiftV)
	}
}
