package spam

// The spam classifier's verdict on one message.
//
// These lived in uicommon, which made every consumer of a scan result import a
// UI package to name its own data, and forced the scan package to alias them
// back out again. They are the classifier's output, so they belong beside it.

// Thresholds are the score boundaries the classifier was configured with.
type Thresholds struct {
	Reject    float64 `json:"reject"`
	AddHeader float64 `json:"add header"`
	Greylist  float64 `json:"greylist"`
}

// Symbol is one named rule that fired, and what it contributed to the score.
type Symbol struct {
	Name        string  `json:"name"`
	Score       float64 `json:"score"`
	Description string  `json:"description"`
}

// Response is the verdict on a single message.
type Response struct {
	Action     string            `json:"action"`
	Score      float64           `json:"score"`
	Required   float64           `json:"required_score"`
	Thresholds Thresholds        `json:"thresholds"`
	Subject    string            `json:"subject"`
	Symbols    map[string]Symbol `json:"symbols"`
}
