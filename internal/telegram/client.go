package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const maxRetries = 3

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute, // 20MB chunks can be slow on limited connections
		},
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

// Upload sends a file to a Telegram channel via sendDocument.
// Automatically retries on 429 (Too Many Requests) using the retry_after value.
func (c *Client) Upload(token string, chatID int64, data []byte, filename string) (*UploadResult, error) {
	convertedChatID := convertChatID(chatID)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		_ = writer.WriteField("chat_id", strconv.FormatInt(convertedChatID, 10))

		part, err := writer.CreateFormFile("document", filename)
		if err != nil {
			return nil, fmt.Errorf("creating form file: %w", err)
		}
		if _, err := part.Write(data); err != nil {
			return nil, fmt.Errorf("writing file data: %w", err)
		}
		writer.Close()

		apiURL := fmt.Sprintf("%s/bot%s/sendDocument", c.baseURL, token)
		req, err := http.NewRequest("POST", apiURL, &body)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("sending document: %w", err)
		}

		if rlErr := parseRateLimitError(resp); rlErr != nil {
			resp.Body.Close()
			if attempt == maxRetries {
				return nil, rlErr
			}
			log.Printf("[telegram] 429 rate limited, waiting %ds before retry (attempt %d/%d)", rlErr.RetryAfter, attempt+1, maxRetries)
			time.Sleep(time.Duration(rlErr.RetryAfter) * time.Second)
			continue
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

	return nil, fmt.Errorf("telegram upload failed after %d retries", maxRetries)
}

// DeleteMessage deletes a message from a Telegram channel.
// Automatically retries on 429 (Too Many Requests).
func (c *Client) DeleteMessage(token string, chatID int64, messageID int64) error {
	convertedChatID := convertChatID(chatID)
	apiURL := fmt.Sprintf("%s/bot%s/deleteMessage?chat_id=%d&message_id=%d",
		c.baseURL, token, convertedChatID, messageID)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := c.httpClient.Get(apiURL)
		if err != nil {
			return fmt.Errorf("deleting message: %w", err)
		}

		if rlErr := parseRateLimitError(resp); rlErr != nil {
			resp.Body.Close()
			if attempt == maxRetries {
				return rlErr
			}
			log.Printf("[telegram] 429 rate limited on deleteMessage, waiting %ds (attempt %d/%d)", rlErr.RetryAfter, attempt+1, maxRetries)
			time.Sleep(time.Duration(rlErr.RetryAfter) * time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("telegram deleteMessage error (status %d): %s", resp.StatusCode, string(respBody))
		}
		resp.Body.Close()
		return nil
	}
	return fmt.Errorf("telegram deleteMessage failed after %d retries", maxRetries)
}

// Download retrieves a file from Telegram by its file_id.
// Automatically retries on 429 (Too Many Requests).
func (c *Client) Download(token string, telegramFileID string) ([]byte, error) {
	// Step 1: Get file path (with retry on 429)
	var filePath string
	getFileURL := fmt.Sprintf("%s/bot%s/getFile?file_id=%s", c.baseURL, token, telegramFileID)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := c.httpClient.Get(getFileURL)
		if err != nil {
			return nil, fmt.Errorf("getting file info: %w", err)
		}

		if rlErr := parseRateLimitError(resp); rlErr != nil {
			resp.Body.Close()
			if attempt == maxRetries {
				return nil, rlErr
			}
			log.Printf("[telegram] 429 rate limited on getFile, waiting %ds (attempt %d/%d)", rlErr.RetryAfter, attempt+1, maxRetries)
			time.Sleep(time.Duration(rlErr.RetryAfter) * time.Second)
			continue
		}

		var fileResp GetFileResponse
		err = json.NewDecoder(resp.Body).Decode(&fileResp)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decoding file info: %w", err)
		}

		if !fileResp.OK || fileResp.Result.FilePath == "" {
			return nil, fmt.Errorf("telegram getFile failed")
		}
		filePath = fileResp.Result.FilePath
		break
	}

	// Step 2: Download the file
	downloadURL := fmt.Sprintf("%s/file/bot%s/%s", c.baseURL, token, filePath)
	dlResp, err := c.httpClient.Get(downloadURL)
	if err != nil {
		return nil, fmt.Errorf("downloading file: %w", err)
	}
	defer dlResp.Body.Close()

	data, err := io.ReadAll(dlResp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading file data: %w", err)
	}

	return data, nil
}

// GenerateChunkFilename generates a filename for a chunk.
func GenerateChunkFilename(fileID uuid.UUID, position int) string {
	return fmt.Sprintf("%s_%d", fileID.String(), position)
}
