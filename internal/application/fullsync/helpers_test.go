package fullsync

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringSlicesEqual_BothEmpty(t *testing.T) {
	result := stringSlicesEqual([]string{}, []string{})
	assert.True(t, result)
}

func TestStringSlicesEqual_BothNil(t *testing.T) {
	result := stringSlicesEqual(nil, nil)
	assert.True(t, result)
}

func TestStringSlicesEqual_SameElements(t *testing.T) {
	a := []string{"KB-1", "KB-2", "KB-3"}
	b := []string{"KB-1", "KB-2", "KB-3"}
	result := stringSlicesEqual(a, b)
	assert.True(t, result)
}

func TestStringSlicesEqual_SameElementsDifferentOrder(t *testing.T) {
	a := []string{"KB-1", "KB-2", "KB-3"}
	b := []string{"KB-3", "KB-1", "KB-2"}
	result := stringSlicesEqual(a, b)
	assert.True(t, result)
}

func TestStringSlicesEqual_DifferentLength(t *testing.T) {
	a := []string{"KB-1", "KB-2"}
	b := []string{"KB-1", "KB-2", "KB-3"}
	result := stringSlicesEqual(a, b)
	assert.False(t, result)
}

func TestStringSlicesEqual_DifferentElements(t *testing.T) {
	a := []string{"KB-1", "KB-2"}
	b := []string{"KB-1", "KB-3"}
	result := stringSlicesEqual(a, b)
	assert.False(t, result)
}

func TestStringSlicesEqual_OneEmpty(t *testing.T) {
	a := []string{"KB-1"}
	b := []string{}
	result := stringSlicesEqual(a, b)
	assert.False(t, result)
}

func TestDifference_NoOverlap(t *testing.T) {
	a := []string{"KB-1", "KB-2"}
	b := []string{"KB-3", "KB-4"}
	result := difference(a, b)
	assert.ElementsMatch(t, []string{"KB-1", "KB-2"}, result)
}

func TestDifference_FullOverlap(t *testing.T) {
	a := []string{"KB-1", "KB-2"}
	b := []string{"KB-1", "KB-2"}
	result := difference(a, b)
	assert.Empty(t, result)
}

func TestDifference_PartialOverlap(t *testing.T) {
	a := []string{"KB-1", "KB-2", "KB-3"}
	b := []string{"KB-2"}
	result := difference(a, b)
	assert.ElementsMatch(t, []string{"KB-1", "KB-3"}, result)
}

func TestDifference_EmptyA(t *testing.T) {
	a := []string{}
	b := []string{"KB-1", "KB-2"}
	result := difference(a, b)
	assert.Empty(t, result)
}

func TestDifference_EmptyB(t *testing.T) {
	a := []string{"KB-1", "KB-2"}
	b := []string{}
	result := difference(a, b)
	assert.ElementsMatch(t, []string{"KB-1", "KB-2"}, result)
}

func TestDifference_BothEmpty(t *testing.T) {
	a := []string{}
	b := []string{}
	result := difference(a, b)
	assert.Empty(t, result)
}

func TestDifference_NilInputs(t *testing.T) {
	result := difference(nil, nil)
	assert.Empty(t, result)

	result = difference(nil, []string{"KB-1"})
	assert.Empty(t, result)

	result = difference([]string{"KB-1"}, nil)
	assert.ElementsMatch(t, []string{"KB-1"}, result)
}
