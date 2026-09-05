package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/google/uuid"
)

const maxRetries = 3

// telegramSleep waits d or until ctx is cancelled. Tests replace it to skip
// rate-limit and backoff waits.
var telegramSleep = func(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type Client struct {
	baseURL        string
	httpClient     *http.Client
	downloadClient *http.Client
}

func newTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 30
	transport.MaxIdleConnsPerHost = 20
	transport.IdleConnTimeout = 90 * time.Second
	transport.ExpectContinueTimeout = time.Second
	transport.ForceAttemptHTTP2 = true
	return transport
}

// newHTTPClient returns a client for Telegram calls. 2 minutes accommodates
// ~20MB chunks on slow connections (e.g. 2 Mbps ~80s) plus Telegram
// processing, TLS negotiation and rate-limit waits.
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   2 * time.Minute,
		Transport: newTransport(),
	}
}

// retryBackoff is the wait before retry number attempt (0-based).
func retryBackoff(attempt int) time.Duration {
	return time.Duration(attempt+1) * 500 * time.Millisecond
}

// NewClient builds a Telegram Bot API client. Uploads and downloads use
// separate connection pools so large sends never starve chunk fetches.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:        baseURL,
		httpClient:     newHTTPClient(),
		downloadClient: newHTTPClient(),
	}
}

// convertChatID converts a regular chat ID to the Telegram Bot API format.
// Prepends -100 to the channel ID: 3696691277 -> -1003696691277
func convertChatID(chatID int64) int64 {
	if chatID < 0 {
		return chatID
	}
	s := fmt.Sprintf("-100%d", chatID)
	result, _ := strconv.ParseInt(s, 10, 64)
	return result
}

// UploadResult holds the result of a sendDocument call.
type UploadResult struct {
	FileID    string
	MessageID int64
}

// parseRateLimitError checks if a response is a 429 and returns the retry_after value.
func parseRateLimitError(resp *http.Response) *RateLimitError {
	if resp.StatusCode != http.StatusTooManyRequests {
		return nil
	}
	var errResp TelegramErrorResponse
	body, _ := io.ReadAll(resp.Body)
	if json.Unmarshal(body, &errResp) == nil && errResp.Parameters.RetryAfter > 0 {
		return &RateLimitError{
			RetryAfter: errResp.Parameters.RetryAfter,
			Message:    fmt.Sprintf("rate limited by Telegram, retry after %ds: %s", errResp.Parameters.RetryAfter, errResp.Description),
		}
	}
	return &RateLimitError{
		RetryAfter: 5,
		Message:    fmt.Sprintf("rate limited by Telegram (429): %s", string(body)),
	}
}

// doWithRetry sends the request built by newReq up to maxRetries+1 times and
// returns the status code with the fully read body. It retries transient
// transport errors and body read failures with backoff, and 429 responses
// after sleeping for Telegram's retry_after. name identifies the call in logs
// and errors. Context cancellation is honoured between retries.
func (c *Client) doWithRetry(ctx context.Context, client *http.Client, name string, newReq func() (*http.Request, error)) (int, []byte, error) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return 0, nil, err
		}

		req, err := newReq()
		if err != nil {
			return 0, nil, fmt.Errorf("creating %s request: %w", name, err)
		}

		resp, err := client.Do(req)
		if err != nil {
			if attempt < maxRetries && isRetryableDownloadError(ctx, err) {
				slog.Warn("telegram transient error on "+name+", retrying", "attempt", attempt+1, "max_retries", maxRetries, "err", err)
				if err := telegramSleep(ctx, retryBackoff(attempt)); err != nil {
					return 0, nil, err
				}
				continue
			}
			return 0, nil, fmt.Errorf("%s request: %w", name, err)
		}

		if rlErr := parseRateLimitError(resp); rlErr != nil {
			resp.Body.Close()
			if attempt == maxRetries {
				return 0, nil, rlErr
			}
			slog.Warn("telegram rate limited on "+name, "retry_after_s", rlErr.RetryAfter, "attempt", attempt+1, "max_retries", maxRetries)
			if err := telegramSleep(ctx, time.Duration(rlErr.RetryAfter)*time.Second); err != nil {
				return 0, nil, err
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			if attempt < maxRetries && isRetryableDownloadError(ctx, err) {
				slog.Warn("telegram transient read error on "+name+", retrying", "attempt", attempt+1, "max_retries", maxRetries, "err", err)
				if err := telegramSleep(ctx, retryBackoff(attempt)); err != nil {
					return 0, nil, err
				}
				continue
			}
			return 0, nil, fmt.Errorf("reading %s response: %w", name, err)
		}

		return resp.StatusCode, body, nil
	}

	return 0, nil, fmt.Errorf("telegram %s failed after %d retries", name, maxRetries)
}

// getWithRetry is doWithRetry for a plain GET.
func (c *Client) getWithRetry(ctx context.Context, client *http.Client, name, url string) (int, []byte, error) {
	return c.doWithRetry(ctx, client, name, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	})
}

func buildUploadEnvelope(chatID int64, filename string) (prefix, suffix []byte, contentType string, err error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if err := writer.WriteField("chat_id", strconv.FormatInt(chatID, 10)); err != nil {
		return nil, nil, "", fmt.Errorf("writing chat_id field: %w", err)
	}
	if _, err := writer.CreateFormFile("document", filename); err != nil {
		return nil, nil, "", fmt.Errorf("creating form file: %w", err)
	}

	contentType = writer.FormDataContentType()
	prefixLen := buf.Len()
	if err := writer.Close(); err != nil {
		return nil, nil, "", fmt.Errorf("closing multipart writer: %w", err)
	}

	all := buf.Bytes()
	prefix = append([]byte(nil), all[:prefixLen]...)
	suffix = append([]byte(nil), all[prefixLen:]...)
	return prefix, suffix, contentType, nil
}

// Upload sends a file to a Telegram channel via sendDocument.
// Automatically retries on 429 (Too Many Requests) using the retry_after value.
func (c *Client) Upload(ctx context.Context, token string, chatID int64, data []byte, filename string) (*UploadResult, error) {
	prefix, suffix, contentType, err := buildUploadEnvelope(convertChatID(chatID), filename)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("%s/bot%s/sendDocument", c.baseURL, token)
	status, body, err := c.doWithRetry(ctx, c.httpClient, "sendDocument", func() (*http.Request, error) {
		reqBody := io.MultiReader(bytes.NewReader(prefix), bytes.NewReader(data), bytes.NewReader(suffix))
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, reqBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", contentType)
		req.ContentLength = int64(len(prefix) + len(data) + len(suffix))
		return req, nil
	})
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("telegram API error (status %d): %s", status, string(body))
	}

	var result SendDocumentResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram sendDocument failed")
	}

	return &UploadResult{
		FileID:    result.Result.Document.FileID,
		MessageID: result.Result.MessageID,
	}, nil
}

// DeleteMessage deletes a message from a Telegram channel.
// Automatically retries on 429 (Too Many Requests).
func (c *Client) DeleteMessage(ctx context.Context, token string, chatID int64, messageID int64) error {
	apiURL := fmt.Sprintf("%s/bot%s/deleteMessage?chat_id=%d&message_id=%d",
		c.baseURL, token, convertChatID(chatID), messageID)

	status, body, err := c.getWithRetry(ctx, c.httpClient, "deleteMessage", apiURL)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("telegram deleteMessage error (status %d): %s", status, string(body))
	}
	return nil
}

// ResolveFileIDByMessage forwards an existing message to the same chat and extracts
// a bot-scoped document file_id, then deletes the forwarded copy.
func (c *Client) ResolveFileIDByMessage(ctx context.Context, token string, chatID int64, messageID int64) (string, error) {
	convertedChatID := convertChatID(chatID)
	apiURL := fmt.Sprintf("%s/bot%s/forwardMessage?chat_id=%d&from_chat_id=%d&message_id=%d&disable_notification=true",
		c.baseURL, token, convertedChatID, convertedChatID, messageID)

	status, body, err := c.getWithRetry(ctx, c.httpClient, "forwardMessage", apiURL)
	if err != nil {
		return "", err
	}

	var forwardResp ForwardMessageResponse
	if err := json.Unmarshal(body, &forwardResp); err != nil {
		return "", fmt.Errorf("decoding forwardMessage response: %w", err)
	}

	if status != http.StatusOK {
		return "", fmt.Errorf("%w: forwardMessage failed (status %d): %s", domain.ErrTelegramResolveFailed, status, string(body))
	}
	if !forwardResp.OK || forwardResp.Result.Document.FileID == "" {
		return "", fmt.Errorf("%w: forwardMessage missing document file_id: %s", domain.ErrTelegramResolveFailed, string(body))
	}

	if err := c.DeleteMessage(ctx, token, chatID, forwardResp.Result.MessageID); err != nil {
		slog.Warn("failed to delete forwarded message", "message_id", forwardResp.Result.MessageID, "err", err)
	}

	return forwardResp.Result.Document.FileID, nil
}

// isRetryableDownloadError reports whether a transport or body read error is
// worth retrying: network errors and truncated bodies, unless ctx is done.
func isRetryableDownloadError(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}

	var netErr net.Error
	return errors.As(err, &netErr)
}

// Download retrieves a file from Telegram by its file_id honoring request cancellation.
// Automatically retries on 429 (Too Many Requests) and transient transport errors.
func (c *Client) Download(ctx context.Context, token string, telegramFileID string) ([]byte, error) {
	// Step 1: resolve the file path.
	getFileURL := fmt.Sprintf("%s/bot%s/getFile?file_id=%s", c.baseURL, token, url.QueryEscape(telegramFileID))
	status, body, err := c.getWithRetry(ctx, c.downloadClient, "getFile", getFileURL)
	if err != nil {
		return nil, err
	}

	var fileResp GetFileResponse
	if err := json.Unmarshal(body, &fileResp); err != nil {
		return nil, fmt.Errorf("decoding file info: %w", err)
	}

	if status != http.StatusOK {
		errMsg := string(body)
		if strings.Contains(strings.ToLower(errMsg), "file is too big") {
			return nil, fmt.Errorf("%w (status %d): %s", domain.ErrTelegramFileTooBig, status, errMsg)
		}
		return nil, fmt.Errorf("%w (status %d): %s", domain.ErrTelegramGetFileFailed, status, errMsg)
	}
	if !fileResp.OK || fileResp.Result.FilePath == "" {
		return nil, fmt.Errorf("%w: %s", domain.ErrTelegramGetFileFailed, string(body))
	}

	// Step 2: download the file bytes.
	downloadURL := fmt.Sprintf("%s/file/bot%s/%s", c.baseURL, token, fileResp.Result.FilePath)
	status, data, err := c.getWithRetry(ctx, c.downloadClient, "file download", downloadURL)
	if err != nil {
		var rlErr *RateLimitError
		if errors.As(err, &rlErr) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadInterrupted, err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("telegram file download error (status %d): %s", status, string(data))
	}

	return data, nil
}

// GenerateChunkFilename generates a filename for a chunk.
func GenerateChunkFilename(fileID uuid.UUID, position int) string {
	return fmt.Sprintf("%s_%d", fileID.String(), position)
}
