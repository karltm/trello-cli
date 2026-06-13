package trello

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Scale-Flow/trello-cli/internal/contract"
)

func (c *Client) ListAttachments(ctx context.Context, cardID string) ([]Attachment, error) {
	var attachments []Attachment
	err := c.Get(ctx, fmt.Sprintf("/1/cards/%s/attachments", cardID), nil, &attachments)
	return attachments, err
}

func (c *Client) AddURLAttachment(ctx context.Context, cardID, urlStr string, name *string) (Attachment, error) {
	queryParams := map[string]string{"url": urlStr}
	if name != nil {
		queryParams["name"] = *name
	}
	var attachment Attachment
	err := c.Post(ctx, fmt.Sprintf("/1/cards/%s/attachments", cardID), queryParams, &attachment)
	return attachment, err
}

func (c *Client) AddFileAttachment(ctx context.Context, cardID, filePath string, name *string) (Attachment, error) {
	queryParams := map[string]string{}
	if name != nil {
		queryParams["name"] = *name
	}
	var attachment Attachment
	err := c.postMultipartFile(ctx, fmt.Sprintf("/1/cards/%s/attachments", cardID), filePath, queryParams, &attachment)
	return attachment, err
}

func (c *Client) DeleteAttachment(ctx context.Context, cardID, attachmentID string) error {
	return c.Delete(ctx, fmt.Sprintf("/1/cards/%s/attachments/%s", cardID, attachmentID), nil)
}

// DownloadAttachment fetches the attachment's metadata and opens its byte
// stream. The caller owns closing the returned ReadCloser.
//
// Trello-hosted uploads cannot be downloaded with the key/token query params
// the rest of the API uses — the download endpoint only accepts credentials in
// an Authorization header. We therefore send the OAuth header, but ONLY to
// Trello hosts, so credentials are never leaked to the third-party URLs that
// link-style attachments point at.
func (c *Client) DownloadAttachment(ctx context.Context, cardID, attachmentID string) (io.ReadCloser, Attachment, error) {
	var att Attachment
	if err := c.Get(ctx, fmt.Sprintf("/1/cards/%s/attachments/%s", cardID, attachmentID), nil, &att); err != nil {
		return nil, Attachment{}, err
	}
	if att.URL == "" {
		return nil, att, contract.NewError(contract.Unsupported, fmt.Sprintf("attachment %s has no downloadable URL", attachmentID))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, att.URL, nil)
	if err != nil {
		return nil, att, contract.NewError(contract.ValidationError, fmt.Sprintf("invalid attachment URL %q: %v", att.URL, err))
	}
	if trustDownloadHost(att.URL) {
		req.Header.Set("Authorization", fmt.Sprintf(`OAuth oauth_consumer_key="%s", oauth_token="%s"`, c.apiKey, c.token))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, att, contract.NewError(contract.HTTPError, fmt.Sprintf("download failed: %v", err))
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer resp.Body.Close()
		return nil, att, mapHTTPError(resp)
	}
	return resp.Body, att, nil
}

// trustDownloadHost decides whether credentials may be attached to a download
// request. It is a package variable solely so tests (whose httptest servers
// never run on a trello.com host) can exercise the authenticated path; in
// production it always points at isTrelloHost.
var trustDownloadHost = isTrelloHost

// isTrelloHost reports whether rawURL points at a Trello-owned host, so we know
// it is safe to attach the user's credentials to the request.
func isTrelloHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "trello.com" || host == "api.trello.com" || strings.HasSuffix(host, ".trello.com")
}

// postMultipartFile handles multipart/form-data file uploads.
func (c *Client) postMultipartFile(ctx context.Context, path, filePath string, params map[string]string, result any) error {
	file, err := os.Open(filePath)
	if err != nil {
		return contract.NewError(contract.FileNotFound, fmt.Sprintf("cannot open file: %s", filePath))
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return contract.NewError(contract.UnknownError, fmt.Sprintf("failed to create form file: %v", err))
	}
	if _, err := io.Copy(part, file); err != nil {
		return contract.NewError(contract.UnknownError, fmt.Sprintf("failed to read file: %v", err))
	}
	for k, v := range params {
		if err := writer.WriteField(k, v); err != nil {
			return contract.NewError(contract.UnknownError, fmt.Sprintf("failed to write form field: %v", err))
		}
	}
	if err := writer.Close(); err != nil {
		return contract.NewError(contract.UnknownError, fmt.Sprintf("failed to finalize multipart body: %v", err))
	}

	fullURL, err := c.buildURL(path, nil)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return contract.NewError(contract.HTTPError, fmt.Sprintf("upload failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return mapHTTPError(resp)
	}
	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return nil
}
