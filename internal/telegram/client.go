package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

// convertChatID converts a regular chat ID to the Telegram Bot API format.
// Formula: chat_id = original - (100 * 10^n) where n = number of digits of abs(original)
func convertChatID(chatID int64) int64 {
	abs := chatID
	if abs < 0 {
		abs = -abs
	}
	n := len(strconv.FormatInt(abs, 10))
	return chatID - int64(100*math.Pow10(n))
}

// Upload sends a file to a Telegram channel via sendDocument.
func (c *Client) Upload(token string, chatID int64, data []byte, filename string) (string, error) {
	convertedChatID := convertChatID(chatID)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	_ = writer.WriteField("chat_id", strconv.FormatInt(convertedChatID, 10))

	part, err := writer.CreateFormFile("document", filename)
	if err != nil {
		return "", fmt.Errorf("creating form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("writing file data: %w", err)
	}
	writer.Close()

	url := fmt.Sprintf("%s/bot%s/sendDocument", c.baseURL, token)
	req, err := http.NewRequest("POST", url, &body)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("telegram API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result SendDocumentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if !result.OK {
		return "", fmt.Errorf("telegram sendDocument failed")
	}

	return result.Result.Document.FileID, nil
}

// Download retrieves a file from Telegram by its file_id.
func (c *Client) Download(token string, telegramFileID string) ([]byte, error) {
	// Step 1: Get file path
	url := fmt.Sprintf("%s/bot%s/getFile?file_id=%s", c.baseURL, token, telegramFileID)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("getting file info: %w", err)
	}
	defer resp.Body.Close()

	var fileResp GetFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&fileResp); err != nil {
		return nil, fmt.Errorf("decoding file info: %w", err)
	}

	if !fileResp.OK || fileResp.Result.FilePath == "" {
		return nil, fmt.Errorf("telegram getFile failed")
	}

	// Step 2: Download the file
	downloadURL := fmt.Sprintf("%s/file/bot%s/%s", c.baseURL, token, fileResp.Result.FilePath)
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
