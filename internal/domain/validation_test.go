package domain

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAndTruncateSummary_UnderLimit(t *testing.T) {
	input := "KB-1: Short title"
	result, validation := ValidateAndTruncateSummary(input)

	assert.Equal(t, input, result)
	assert.False(t, validation.WasTruncated)
	assert.Equal(t, len(input), validation.OriginalLen)
	assert.Equal(t, len(input), validation.TruncatedLen)
	assert.Equal(t, JiraSummaryMaxLength, validation.MaxLength)
}

func TestValidateAndTruncateSummary_ExactLimit(t *testing.T) {
	// Create a string exactly at the limit
	input := strings.Repeat("a", JiraSummaryMaxLength)
	result, validation := ValidateAndTruncateSummary(input)

	assert.Equal(t, input, result)
	assert.False(t, validation.WasTruncated)
	assert.Equal(t, JiraSummaryMaxLength, validation.OriginalLen)
}

func TestValidateAndTruncateSummary_OverLimit(t *testing.T) {
	// Create a string over the limit
	input := strings.Repeat("a", JiraSummaryMaxLength+100)
	result, validation := ValidateAndTruncateSummary(input)

	assert.True(t, validation.WasTruncated)
	assert.Equal(t, JiraSummaryMaxLength+100, validation.OriginalLen)
	assert.LessOrEqual(t, len(result), JiraSummaryMaxLength)
	assert.True(t, strings.HasSuffix(result, "..."))
}

func TestValidateAndTruncateSummary_Unicode(t *testing.T) {
	// Test with unicode characters (each emoji is multiple bytes but 1 rune)
	input := strings.Repeat("🎉", JiraSummaryMaxLength+10)
	result, validation := ValidateAndTruncateSummary(input)

	assert.True(t, validation.WasTruncated)
	// Result should be measured in runes, not bytes
	runeCount := len([]rune(result))
	assert.LessOrEqual(t, runeCount, JiraSummaryMaxLength)
}

func TestValidateAndTruncateSummary_Empty(t *testing.T) {
	result, validation := ValidateAndTruncateSummary("")

	assert.Equal(t, "", result)
	assert.False(t, validation.WasTruncated)
	assert.Equal(t, 0, validation.OriginalLen)
}

func TestValidateAndTruncateDescription_UnderLimit(t *testing.T) {
	input := "Short description"
	result, validation := ValidateAndTruncateDescription(input)

	assert.Equal(t, input, result)
	assert.False(t, validation.WasTruncated)
	assert.Equal(t, JiraDescriptionMaxLength, validation.MaxLength)
}

func TestValidateAndTruncateDescription_OverLimit(t *testing.T) {
	// Create a string over the limit
	input := strings.Repeat("a", JiraDescriptionMaxLength+1000)
	result, validation := ValidateAndTruncateDescription(input)

	assert.True(t, validation.WasTruncated)
	assert.Equal(t, JiraDescriptionMaxLength+1000, validation.OriginalLen)
	assert.LessOrEqual(t, len(result), JiraDescriptionMaxLength)
	assert.True(t, strings.HasSuffix(result, "[Content truncated: exceeded Jira limit]"))
}

func TestValidateAndTruncateDescription_ExactLimit(t *testing.T) {
	input := strings.Repeat("b", JiraDescriptionMaxLength)
	result, validation := ValidateAndTruncateDescription(input)

	assert.Equal(t, input, result)
	assert.False(t, validation.WasTruncated)
}

func TestFieldValidation_WarningMessage(t *testing.T) {
	tests := []struct {
		name       string
		validation FieldValidation
		expected   string
	}{
		{
			name: "not truncated",
			validation: FieldValidation{
				FieldName:    "Summary",
				OriginalLen:  100,
				TruncatedLen: 100,
				WasTruncated: false,
				MaxLength:    255,
			},
			expected: "",
		},
		{
			name: "truncated",
			validation: FieldValidation{
				FieldName:    "Summary",
				OriginalLen:  300,
				TruncatedLen: 255,
				WasTruncated: true,
				MaxLength:    255,
			},
			expected: "Summary truncated: 300 chars exceeds limit of 255 chars",
		},
		{
			name: "description truncated",
			validation: FieldValidation{
				FieldName:    "Description",
				OriginalLen:  40000,
				TruncatedLen: 32767,
				WasTruncated: true,
				MaxLength:    32767,
			},
			expected: "Description truncated: 40000 chars exceeds limit of 32767 chars",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.validation.WarningMessage())
		})
	}
}

func TestValidationResult_HasWarnings(t *testing.T) {
	tests := []struct {
		name     string
		result   ValidationResult
		expected bool
	}{
		{
			name:     "no validations",
			result:   ValidationResult{},
			expected: false,
		},
		{
			name: "no truncation",
			result: ValidationResult{
				Summary:     &FieldValidation{WasTruncated: false},
				Description: &FieldValidation{WasTruncated: false},
			},
			expected: false,
		},
		{
			name: "summary truncated",
			result: ValidationResult{
				Summary:     &FieldValidation{WasTruncated: true},
				Description: &FieldValidation{WasTruncated: false},
			},
			expected: true,
		},
		{
			name: "description truncated",
			result: ValidationResult{
				Summary:     &FieldValidation{WasTruncated: false},
				Description: &FieldValidation{WasTruncated: true},
			},
			expected: true,
		},
		{
			name: "both truncated",
			result: ValidationResult{
				Summary:     &FieldValidation{WasTruncated: true},
				Description: &FieldValidation{WasTruncated: true},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.result.HasWarnings())
		})
	}
}

func TestValidationResult_Warnings(t *testing.T) {
	result := ValidationResult{
		Summary: &FieldValidation{
			FieldName:    "Summary",
			OriginalLen:  300,
			WasTruncated: true,
			MaxLength:    255,
		},
		Description: &FieldValidation{
			FieldName:    "Description",
			OriginalLen:  40000,
			WasTruncated: true,
			MaxLength:    32767,
		},
	}

	warnings := result.Warnings()
	assert.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], "Summary")
	assert.Contains(t, warnings[1], "Description")
}

func TestFieldValidator_ValidateTask(t *testing.T) {
	var logBuf bytes.Buffer
	validator := NewFieldValidator(&logBuf)

	task := &TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: Frontmatter{
			Title:      strings.Repeat("x", JiraSummaryMaxLength+50),
			JiraParent: "GUARD-100",
		},
		Description: "Normal description",
	}

	result := validator.ValidateTask(task)

	// Title should be truncated
	assert.True(t, result.Summary.WasTruncated)
	assert.LessOrEqual(t, len(task.Frontmatter.Title), JiraSummaryMaxLength)

	// Description should not be truncated
	assert.False(t, result.Description.WasTruncated)

	// Log should contain warning
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "WARNING")
	assert.Contains(t, logOutput, "Summary truncated")
	assert.Contains(t, logOutput, "/tasks/test.md")
}

func TestFieldValidator_ValidateTask_NoLogger(t *testing.T) {
	validator := NewFieldValidator(nil)

	task := &TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: Frontmatter{
			Title:      strings.Repeat("x", JiraSummaryMaxLength+50),
			JiraParent: "GUARD-100",
		},
		Description: "Normal description",
	}

	// Should not panic with nil logger
	result := validator.ValidateTask(task)
	require.NotNil(t, result)
	assert.True(t, result.Summary.WasTruncated)
}

func TestFieldValidator_ValidateTask_BothFieldsTruncated(t *testing.T) {
	var logBuf bytes.Buffer
	validator := NewFieldValidator(&logBuf)

	task := &TaskFile{
		Path: "/tasks/long.md",
		Frontmatter: Frontmatter{
			Title:      strings.Repeat("T", JiraSummaryMaxLength+100),
			JiraParent: "GUARD-100",
		},
		Description: strings.Repeat("D", JiraDescriptionMaxLength+1000),
	}

	result := validator.ValidateTask(task)

	// Both should be truncated
	assert.True(t, result.Summary.WasTruncated)
	assert.True(t, result.Description.WasTruncated)

	// Task fields should be modified in place
	assert.LessOrEqual(t, len(task.Frontmatter.Title), JiraSummaryMaxLength)
	assert.LessOrEqual(t, len(task.Description), JiraDescriptionMaxLength)

	// Log should contain both warnings
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "Summary truncated")
	assert.Contains(t, logOutput, "Description truncated")
}

func TestTruncateToRunes_PreservesUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		n        int
		expected string
	}{
		{
			name:     "ascii",
			input:    "hello world",
			n:        5,
			expected: "hello",
		},
		{
			name:     "emoji",
			input:    "🎉🎊🎁🎈",
			n:        2,
			expected: "🎉🎊",
		},
		{
			name:     "japanese",
			input:    "日本語テスト",
			n:        3,
			expected: "日本語",
		},
		{
			name:     "mixed",
			input:    "Hello 世界!",
			n:        8,
			expected: "Hello 世界",
		},
		{
			name:     "zero",
			input:    "hello",
			n:        0,
			expected: "",
		},
		{
			name:     "negative",
			input:    "hello",
			n:        -1,
			expected: "",
		},
		{
			name:     "longer than input",
			input:    "hi",
			n:        100,
			expected: "hi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateToRunes(tt.input, tt.n)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Fuzz tests for validation

func FuzzValidateAndTruncateSummary(f *testing.F) {
	f.Add("Short title")
	f.Add(strings.Repeat("a", JiraSummaryMaxLength))
	f.Add(strings.Repeat("b", JiraSummaryMaxLength+100))
	f.Add("日本語タイトル")
	f.Add("")
	f.Add("🎉🎊🎁")

	f.Fuzz(func(t *testing.T, input string) {
		result, validation := ValidateAndTruncateSummary(input)

		// Result should never exceed max length in runes
		runeCount := len([]rune(result))
		if runeCount > JiraSummaryMaxLength {
			t.Errorf("Result rune count %d exceeds max %d", runeCount, JiraSummaryMaxLength)
		}

		// If input was under limit, result should match
		inputRunes := len([]rune(input))
		if inputRunes <= JiraSummaryMaxLength && result != input {
			t.Errorf("Input under limit but result differs")
		}

		// Validation should be consistent
		if validation.WasTruncated && inputRunes <= JiraSummaryMaxLength {
			t.Error("WasTruncated true but input was under limit")
		}
		if !validation.WasTruncated && inputRunes > JiraSummaryMaxLength {
			t.Error("WasTruncated false but input was over limit")
		}
	})
}

func FuzzValidateAndTruncateDescription(f *testing.F) {
	f.Add("Short description")
	f.Add(strings.Repeat("a", JiraDescriptionMaxLength))
	f.Add(strings.Repeat("b", JiraDescriptionMaxLength+100))
	f.Add("日本語説明文")
	f.Add("")

	f.Fuzz(func(t *testing.T, input string) {
		result, validation := ValidateAndTruncateDescription(input)

		// Result should never exceed max length in runes
		runeCount := len([]rune(result))
		if runeCount > JiraDescriptionMaxLength {
			t.Errorf("Result rune count %d exceeds max %d", runeCount, JiraDescriptionMaxLength)
		}

		// Validation consistency
		inputRunes := len([]rune(input))
		if validation.WasTruncated && inputRunes <= JiraDescriptionMaxLength {
			t.Error("WasTruncated true but input was under limit")
		}
	})
}
