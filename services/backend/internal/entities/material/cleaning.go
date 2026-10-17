package material

// Cleaning is separate from OCR status: a cleaning failure never loses OCR text.
type Cleaning struct {
	Status       string `json:"status" enums:"pending,processing,ready,failed"`
	Attempts     int    `json:"attempts"`
	CleanedText  string `json:"cleaned_text"`
	S3CleanedKey string `json:"s3_cleaned_key"`
	S3ReportKey  string `json:"s3_report_key"`
	ErrorMessage string `json:"error_message"`
	UpdatedAt    int64  `json:"updated_at"`
}

type CleaningJob struct {
	MaterialID string
	Token      string
	Attempts   int
}
