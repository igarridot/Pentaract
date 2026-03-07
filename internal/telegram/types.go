package telegram

type SendDocumentResponse struct {
	OK     bool `json:"ok"`
	Result struct {
		MessageID int64 `json:"message_id"`
		Document  struct {
			FileID string `json:"file_id"`
		} `json:"document"`
	} `json:"result"`
}

type GetFileResponse struct {
	OK     bool `json:"ok"`
	Result struct {
		FilePath string `json:"file_path"`
	} `json:"result"`
}

// TelegramErrorResponse represents a Telegram API error with retry_after.
type TelegramErrorResponse struct {
	OK          bool `json:"ok"`
	ErrorCode   int  `json:"error_code"`
	Description string `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// RateLimitError is returned when Telegram responds with 429 Too Many Requests.
type RateLimitError struct {
	RetryAfter int
	Message    string
}

func (e *RateLimitError) Error() string {
	return e.Message
}
