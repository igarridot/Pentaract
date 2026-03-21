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

var telegramSleep = time.Sleep

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

// newUploadClient returns an HTTP client for uploads (large body sends).
// 2 minutes accommodates ~20MB chunks on slow connections (e.g. 2 Mbps ~80s)
// plus Telegram processing, TLS negotiation and rate-limit waits.
func newUploadClient() *http.Client {
	return &http.Client{
		Timeout:   2 * time.Minute,
		Transport: newTransport(),
	}
}

// newDownloadClient returns an HTTP client for downloads and API calls
// where a 2-minute timeout is more than enough for ~20MB chunks.
func newDownloadClient() *http.Client {
	return &http.Client{
		Timeout:   2 * time.Minute,
		Transport: newTransport(),
	}
}

// retryBackoff sleeps with exponential backoff before a retry attempt.
func retryBackoff(ctx context.Context, attempt int) {
	backoff := time.Duration(attempt+1) * 500 * time.Millisecond
	select {
	case <-ctx.Done():
	case <-time.After(backoff):
	}
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:        baseURL,
		httpClient:     newUploadClient(),
		downloadClient: newDownloadClient(),
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

// doWithRateLimitRetry executes do() up to maxRetries+1 times, handling 429 rate-limit
// responses by sleeping for the retry_after duration and retrying. The name parameter
// is used in log messages to identify the operation.
// S6: respects context cancellation between retries.
func (c *Client) doWithRateLimitRetry(ctx context.Context, name string, do func() (*http.Response, error)) (*http.Response, error) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resp, err := do()
		if err != nil {
			return nil, err
		}

		if rlErr := parseRateLimitError(resp); rlErr != nil {
			resp.Body.Close()
			if attempt == maxRetries {
				return nil, rlErr
			}
			slog.Warn("telegram rate limited on "+name, "retry_after_s", rlErr.RetryAfter, "attempt", attempt+1, "max_retries", maxRetries)
			sleepDur := time.Duration(rlErr.RetryAfter) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(sleepDur):
			}
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("telegram %s failed after %d retries", name, maxRetries)
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
// S6: accepts context for cancellation propagation.
// Automatically retries on 429 (Too Many Requests) using the retry_after value.
func (c *Client) Upload(ctx context.Context, token string, chatID int64, data []byte, filename string) (*UploadResult, error) {
	convertedChatID := convertChatID(chatID)
	prefix, suffix, contentType, err := buildUploadEnvelope(convertedChatID, filename)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("%s/bot%s/sendDocument", c.baseURL, token)

	resp, err := c.doWithRateLimitRetry(ctx, "sendDocument", func() (*http.Response, error) {
		body := io.MultiReader(bytes.NewReader(prefix), bytes.NewReader(data), bytes.NewReader(suffix))
		req, err := http.NewRequestWithContext(ctx, "POST", apiURL, body)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", contentType)
		req.ContentLength = int64(len(prefix) + len(data) + len(suffix))

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("sending document: %w", err)
		}
		return resp, nil
	})
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("telegram API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result SendDocumentResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	if err != nil {
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
func (c *Client) DeleteMessage(token string, chatID int64, messageID int64) error {
	convertedChatID := convertChatID(chatID)
	apiURL := fmt.Sprintf("%s/bot%s/deleteMessage?chat_id=%d&message_id=%d",
		c.baseURL, token, convertedChatID, messageID)

	resp, err := c.doWithRateLimitRetry(context.Background(), "deleteMessage", func() (*http.Response, error) {
		resp, err := c.httpClient.Get(apiURL)
		if err != nil {
			return nil, fmt.Errorf("deleting message: %w", err)
		}
		return resp, nil
	})
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return fmt.Errorf("telegram deleteMessage error (status %d): %s", resp.StatusCode, string(respBody))
	}
	resp.Body.Close()
	return nil
}

// ResolveFileIDByMessage forwards an existing message to the same chat and extracts
// a bot-scoped document file_id, then deletes the forwarded copy.
func (c *Client) ResolveFileIDByMessage(ctx context.Context, token string, chatID int64, messageID int64) (string, error) {
	convertedChatID := convertChatID(chatID)
	apiURL := fmt.Sprintf("%s/bot%s/forwardMessage?chat_id=%d&from_chat_id=%d&message_id=%d&disable_notification=true",
		c.baseURL, token, convertedChatID, convertedChatID, messageID)

	resp, err := c.doWithRateLimitRetry(ctx, "forwardMessage", func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating forwardMessage request: %w", err)
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("forwarding message: %w", err)
		}
		return resp, nil
	})
	if err != nil {
		return "", err
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("reading forwardMessage response: %w", err)
	}

	var forwardResp ForwardMessageResponse
	if err := json.Unmarshal(body, &forwardResp); err != nil {
		return "", fmt.Errorf("decoding forwardMessage response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: forwardMessage failed (status %d): %s", domain.ErrTelegramResolveFailed, resp.StatusCode, string(body))
	}
	if !forwardResp.OK || forwardResp.Result.Document.FileID == "" {
		return "", fmt.Errorf("%w: forwardMessage missing document file_id: %s", domain.ErrTelegramResolveFailed, string(body))
	}

	if err := c.DeleteMessage(token, chatID, forwardResp.Result.MessageID); err != nil {
		slog.Warn("failed to delete forwarded message", "message_id", forwardResp.Result.MessageID, "err", err)
	}

	return forwardResp.Result.Document.FileID, nil
}

func isRetryableDownloadError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}

	var netErr net.Error
	return errors.As(err, &netErr)
}

// Download retrieves a file from Telegram by its file_id honoring request cancellation.
// Automatically retries on 429 (Too Many Requests).
func (c *Client) Download(ctx context.Context, token string, telegramFileID string) ([]byte, error) {
	// Step 1: Get file path (with retry on 429 and transient errors)
	getFileURL := fmt.Sprintf("%s/bot%s/getFile?file_id=%s", c.baseURL, token, url.QueryEscape(telegramFileID))

	resp, err := c.doWithRateLimitRetry(ctx, "getFile", func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, getFileURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating getFile request: %w", err)
		}
		resp, err := c.downloadClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("getting file info: %w", err)
		}
		return resp, nil
	})
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("reading getFile response: %w", err)
	}

	var fileResp GetFileResponse
	if err := json.Unmarshal(body, &fileResp); err != nil {
		return nil, fmt.Errorf("decoding file info: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		errMsg := string(body)
		if strings.Contains(strings.ToLower(errMsg), "file is too big") {
			return nil, fmt.Errorf("%w (status %d): %s", domain.ErrTelegramFileTooBig, resp.StatusCode, errMsg)
		}
		return nil, fmt.Errorf("%w (status %d): %s", domain.ErrTelegramGetFileFailed, resp.StatusCode, errMsg)
	}
	if !fileResp.OK || fileResp.Result.FilePath == "" {
		return nil, fmt.Errorf("%w: %s", domain.ErrTelegramGetFileFailed, string(body))
	}

	// Step 2: Download the file (with retry on 429 and transient errors)
	downloadURL := fmt.Sprintf("%s/file/bot%s/%s", c.baseURL, token, fileResp.Result.FilePath)
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating file download request: %w", err)
		}
		dlResp, err := c.downloadClient.Do(req)
		if err != nil {
			if attempt < maxRetries && isRetryableDownloadError(ctx, err) {
				slog.Warn("transient file download error, retrying", "attempt", attempt+1, "max_retries", maxRetries, "err", err)
				retryBackoff(ctx, attempt)
				continue
			}
			return nil, fmt.Errorf("%w: %v", domain.ErrDownloadInterrupted, err)
		}

		if rlErr := parseRateLimitError(dlResp); rlErr != nil {
			dlResp.Body.Close()
			if attempt == maxRetries {
				return nil, rlErr
			}
			slog.Warn("telegram rate limited on file download", "retry_after_s", rlErr.RetryAfter, "attempt", attempt+1, "max_retries", maxRetries)
			telegramSleep(time.Duration(rlErr.RetryAfter) * time.Second)
			continue
		}

		if dlResp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(dlResp.Body)
			dlResp.Body.Close()
			return nil, fmt.Errorf("telegram file download error (status %d): %s", dlResp.StatusCode, string(respBody))
		}

		data, err := io.ReadAll(dlResp.Body)
		dlResp.Body.Close()
		if err != nil {
			if attempt < maxRetries && isRetryableDownloadError(ctx, err) {
				slog.Warn("transient file read error, retrying", "attempt", attempt+1, "max_retries", maxRetries, "err", err)
				retryBackoff(ctx, attempt)
				continue
			}
			return nil, fmt.Errorf("%w: %v", domain.ErrDownloadInterrupted, err)
		}

		return data, nil
	}

	return nil, fmt.Errorf("telegram file download failed after %d retries", maxRetries)
}

// GenerateChunkFilename generates a filename for a chunk.
func GenerateChunkFilename(fileID uuid.UUID, position int) string {
	return fmt.Sprintf("%s_%d", fileID.String(), position)
}
