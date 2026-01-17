package domain

import (
	"io"
	"unicode/utf8"
)

// FieldValidation contains the result of validating a field.
type FieldValidation struct {
	FieldName     string
	OriginalLen   int
	TruncatedLen  int
	WasTruncated  bool
	MaxLength     int
	TruncatedText string
}

// ValidationResult contains all field validations for a task.
type ValidationResult struct {
	Summary     *FieldValidation
	Description *FieldValidation
}

// HasWarnings returns true if any field was truncated.
func (v *ValidationResult) HasWarnings() bool {
	return (v.Summary != nil && v.Summary.WasTruncated) ||
		(v.Description != nil && v.Description.WasTruncated)
}

// Warnings returns a slice of warning messages for truncated fields.
func (v *ValidationResult) Warnings() []string {
	var warnings []string
	if v.Summary != nil && v.Summary.WasTruncated {
		warnings = append(warnings, v.Summary.WarningMessage())
	}
	if v.Description != nil && v.Description.WasTruncated {
		warnings = append(warnings, v.Description.WarningMessage())
	}
	return warnings
}

// WarningMessage returns a human-readable warning message.
func (f *FieldValidation) WarningMessage() string {
	if !f.WasTruncated {
		return ""
	}
	return f.FieldName + " truncated: " +
		itoa(f.OriginalLen) + " chars exceeds limit of " +
		itoa(f.MaxLength) + " chars"
}

// itoa converts an int to string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}

// ValidateAndTruncateSummary validates and truncates a summary field if needed.
// Returns the (possibly truncated) value and validation info.
func ValidateAndTruncateSummary(summary string) (string, *FieldValidation) {
	return validateAndTruncate(summary, "Summary", JiraSummaryMaxLength, "...")
}

// ValidateAndTruncateDescription validates and truncates a description field if needed.
// Returns the (possibly truncated) value and validation info.
func ValidateAndTruncateDescription(description string) (string, *FieldValidation) {
	return validateAndTruncate(description, "Description", JiraDescriptionMaxLength,
		"\n\n[Content truncated: exceeded Jira limit]")
}

// validateAndTruncate performs validation and truncation for any text field.
func validateAndTruncate(text, fieldName string, maxLen int, suffix string) (string, *FieldValidation) {
	originalLen := utf8.RuneCountInString(text)

	validation := &FieldValidation{
		FieldName:   fieldName,
		OriginalLen: originalLen,
		MaxLength:   maxLen,
	}

	if originalLen <= maxLen {
		validation.TruncatedLen = originalLen
		validation.WasTruncated = false
		validation.TruncatedText = text
		return text, validation
	}

	// Truncate to maxLen - suffix length, ensuring we don't break UTF-8
	targetLen := maxLen - utf8.RuneCountInString(suffix)
	if targetLen < 0 {
		targetLen = 0
	}

	truncated := truncateToRunes(text, targetLen) + suffix
	validation.TruncatedLen = utf8.RuneCountInString(truncated)
	validation.WasTruncated = true
	validation.TruncatedText = truncated

	return truncated, validation
}

// truncateToRunes truncates a string to n runes, preserving UTF-8 encoding.
func truncateToRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}

	runes := 0
	for i := range s {
		if runes >= n {
			return s[:i]
		}
		runes++
	}
	return s
}

// FieldValidator provides methods for validating task fields with logging.
type FieldValidator struct {
	logger io.Writer
}

// NewFieldValidator creates a new FieldValidator with optional logger.
// If logger is nil, warnings are silently discarded.
func NewFieldValidator(logger io.Writer) *FieldValidator {
	return &FieldValidator{logger: logger}
}

// ValidateTask validates and truncates all relevant fields in a TaskFile.
// It modifies the task in place and returns the validation result.
func (v *FieldValidator) ValidateTask(task *TaskFile) *ValidationResult {
	result := &ValidationResult{}

	// Validate summary (title)
	truncatedSummary, summaryValidation := ValidateAndTruncateSummary(task.Frontmatter.Title)
	task.Frontmatter.Title = truncatedSummary
	result.Summary = summaryValidation

	// Validate description
	truncatedDesc, descValidation := ValidateAndTruncateDescription(task.Description)
	task.Description = truncatedDesc
	result.Description = descValidation

	// Log warnings if logger is configured
	if v.logger != nil && result.HasWarnings() {
		for _, warning := range result.Warnings() {
			v.logger.Write([]byte("WARNING: " + task.Path + ": " + warning + "\n"))
		}
	}

	return result
}
