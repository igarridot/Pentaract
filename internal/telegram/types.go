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
