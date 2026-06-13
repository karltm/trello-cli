package trello

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadAttachmentSendsAuthHeader(t *testing.T) {
	const fileBody = "binary-bytes-here"
	var downloadAuth string
	var downloadAuthSeen bool

	mux := http.NewServeMux()
	mux.HandleFunc("/1/cards/c1/attachments/a1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("metadata method = %s, want GET", r.Method)
		}
		downloadURL := "http://" + r.Host + "/download/report.pdf"
		if err := json.NewEncoder(w).Encode(map[string]any{
			"id": "a1", "name": "report.pdf", "url": downloadURL, "bytes": len(fileBody), "mimeType": "application/pdf", "isUpload": true,
		}); err != nil {
			t.Fatalf("Encode() error: %v", err)
		}
	})
	mux.HandleFunc("/download/report.pdf", func(w http.ResponseWriter, r *http.Request) {
		downloadAuth = r.Header.Get("Authorization")
		downloadAuthSeen = true
		// The download endpoint must NOT be authenticated via query params.
		if r.URL.Query().Get("key") != "" || r.URL.Query().Get("token") != "" {
			t.Errorf("download URL carried key/token query params: %s", r.URL.RawQuery)
		}
		if _, err := io.WriteString(w, fileBody); err != nil {
			t.Fatalf("WriteString() error: %v", err)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	// httptest never runs on a trello.com host, so trust the test server for
	// the duration of this test to exercise the authenticated download path.
	orig := trustDownloadHost
	trustDownloadHost = func(string) bool { return true }
	defer func() { trustDownloadHost = orig }()

	client := NewClient(server.URL, "mykey", "mytoken", DefaultClientOptions())
	body, att, err := client.DownloadAttachment(context.Background(), "c1", "a1")
	if err != nil {
		t.Fatalf("DownloadAttachment() error: %v", err)
	}
	defer body.Close()

	if att.Name != "report.pdf" {
		t.Errorf("att.Name = %q, want report.pdf", att.Name)
	}
	if !downloadAuthSeen {
		t.Fatal("download endpoint was never reached")
	}
	wantAuth := `OAuth oauth_consumer_key="mykey", oauth_token="mytoken"`
	if downloadAuth != wantAuth {
		t.Errorf("download Authorization = %q, want %q", downloadAuth, wantAuth)
	}
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll() error: %v", err)
	}
	if string(data) != fileBody {
		t.Errorf("downloaded body = %q, want %q", data, fileBody)
	}
}

func TestIsTrelloHost(t *testing.T) {
	cases := map[string]bool{
		"https://trello.com/1/cards/x/attachments/y/download/f.pdf": true,
		"https://api.trello.com/1/cards/x":                          true,
		"https://trello-attachments.s3.amazonaws.com/foo":           false,
		"https://evil.com/trello.com":                               false,
		"https://nottrello.com/file":                                false,
		"":                                                          false,
	}
	for in, want := range cases {
		if got := isTrelloHost(in); got != want {
			t.Errorf("isTrelloHost(%q) = %v, want %v", in, got, want)
		}
	}
}
