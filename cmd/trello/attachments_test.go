package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Scale-Flow/trello-cli/internal/credentials"
	"github.com/Scale-Flow/trello-cli/internal/trello"
)

func TestAttachmentsListCommand(t *testing.T) {
	setupTestAuth(t)
	credStore.Set("default", credentials.Credentials{APIKey: "k", Token: "t", AuthMode: "manual"})
	apiClient = &mockAPI{
		listAttachmentsFn: func(ctx context.Context, cardID string) ([]trello.Attachment, error) {
			if cardID != "c1" {
				t.Errorf("card ID = %q, want c1", cardID)
			}
			return []trello.Attachment{{ID: "a1", Name: "file.txt"}}, nil
		},
	}

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"attachments", "list", "--card", "c1"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("attachments list failed: %v", err)
	}

	var envelope map[string]any
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	data := envelope["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(data))
	}
}

func TestAttachmentsAddFileCommand(t *testing.T) {
	setupTestAuth(t)
	credStore.Set("default", credentials.Credentials{APIKey: "k", Token: "t", AuthMode: "manual"})
	file, err := os.CreateTemp(t.TempDir(), "attachment-*.txt")
	if err != nil {
		t.Fatalf("CreateTemp() error: %v", err)
	}
	if _, err := file.WriteString("hello"); err != nil {
		t.Fatalf("WriteString() error: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	name := "renamed.txt"
	apiClient = &mockAPI{
		addFileAttachmentFn: func(ctx context.Context, cardID, filePath string, gotName *string) (trello.Attachment, error) {
			if cardID != "c1" || filePath != file.Name() {
				t.Fatalf("card/path = %q/%q", cardID, filePath)
			}
			if gotName == nil || *gotName != name {
				t.Fatalf("name = %v", gotName)
			}
			return trello.Attachment{ID: "a1", Name: name}, nil
		},
	}

	if err := executeRootArgs("attachments", "add-file", "--card", "c1", "--path", file.Name(), "--name", name); err != nil {
		t.Fatalf("attachments add-file failed: %v", err)
	}
}

func TestAttachmentsAddFileMissingPath(t *testing.T) {
	setupTestAuth(t)
	credStore.Set("default", credentials.Credentials{APIKey: "k", Token: "t", AuthMode: "manual"})
	assertContractCode(t, executeRootArgs("attachments", "add-file", "--card", "c1", "--path", "/nope"), "FILE_NOT_FOUND")
}

func TestAttachmentsAddURLCommand(t *testing.T) {
	setupTestAuth(t)
	credStore.Set("default", credentials.Credentials{APIKey: "k", Token: "t", AuthMode: "manual"})
	name := "Reference"
	apiClient = &mockAPI{
		addURLAttachmentFn: func(ctx context.Context, cardID, urlStr string, gotName *string) (trello.Attachment, error) {
			if cardID != "c1" || urlStr != "https://example.com" {
				t.Fatalf("card/url = %q/%q", cardID, urlStr)
			}
			if gotName == nil || *gotName != name {
				t.Fatalf("name = %v", gotName)
			}
			return trello.Attachment{ID: "a1", Name: name}, nil
		},
	}

	if err := executeRootArgs("attachments", "add-url", "--card", "c1", "--url", "https://example.com", "--name", name); err != nil {
		t.Fatalf("attachments add-url failed: %v", err)
	}
}

func TestAttachmentsAddURLInvalidURL(t *testing.T) {
	setupTestAuth(t)
	credStore.Set("default", credentials.Credentials{APIKey: "k", Token: "t", AuthMode: "manual"})
	assertContractCode(t, executeRootArgs("attachments", "add-url", "--card", "c1", "--url", "notaurl"), "VALIDATION_ERROR")
}

func TestAttachmentsDownloadCommand(t *testing.T) {
	setupTestAuth(t)
	credStore.Set("default", credentials.Credentials{APIKey: "k", Token: "t", AuthMode: "manual"})
	dir := t.TempDir()
	apiClient = &mockAPI{
		downloadAttachmentFn: func(ctx context.Context, cardID, attachmentID string) (io.ReadCloser, trello.Attachment, error) {
			if cardID != "c1" || attachmentID != "a1" {
				t.Fatalf("card/attachment = %q/%q", cardID, attachmentID)
			}
			return io.NopCloser(strings.NewReader("payload")), trello.Attachment{ID: "a1", Name: "report.pdf"}, nil
		},
	}

	out := filepath.Join(dir, "saved.pdf")
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"attachments", "download", "--card", "c1", "--attachment", "a1", "--out", out})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("attachments download failed: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("file contents = %q, want payload", got)
	}

	var envelope map[string]any
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	data := envelope["data"].(map[string]any)
	if data["path"] != out || data["bytes"].(float64) != 7 {
		t.Fatalf("data = %+v", data)
	}
}

func TestAttachmentsDownloadDefaultsToAttachmentName(t *testing.T) {
	setupTestAuth(t)
	credStore.Set("default", credentials.Credentials{APIKey: "k", Token: "t", AuthMode: "manual"})
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	apiClient = &mockAPI{
		downloadAttachmentFn: func(ctx context.Context, cardID, attachmentID string) (io.ReadCloser, trello.Attachment, error) {
			// Server-supplied name with a path component must be reduced to its base.
			return io.NopCloser(strings.NewReader("data")), trello.Attachment{ID: "a1", Name: "../escape.txt"}, nil
		},
	}

	if err := executeRootArgs("attachments", "download", "--card", "c1", "--attachment", "a1"); err != nil {
		t.Fatalf("attachments download failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "escape.txt")); err != nil {
		t.Fatalf("expected escape.txt in cwd: %v", err)
	}
}

func TestAttachmentsDeleteCommand(t *testing.T) {
	setupTestAuth(t)
	credStore.Set("default", credentials.Credentials{APIKey: "k", Token: "t", AuthMode: "manual"})
	apiClient = &mockAPI{
		deleteAttachmentFn: func(ctx context.Context, cardID, attachmentID string) error {
			if cardID != "c1" || attachmentID != "a1" {
				t.Fatalf("card/attachment = %q/%q", cardID, attachmentID)
			}
			return nil
		},
	}

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"attachments", "delete", "--card", "c1", "--attachment", "a1"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("attachments delete failed: %v", err)
	}

	var envelope map[string]any
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	data := envelope["data"].(map[string]any)
	if data["deleted"] != true || data["id"] != "a1" {
		t.Fatalf("data = %+v", data)
	}
}
