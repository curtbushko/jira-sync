package pull

import "sort"

// stringSlicesEqual compares two string slices (order-independent).
// Returns true if both slices contain the same elements.
func stringSlicesEqual(slice1, slice2 []string) bool {
	if len(slice1) != len(slice2) {
		return false
	}

	// Make copies to avoid modifying original slices
	sorted1 := make([]string, len(slice1))
	sorted2 := make([]string, len(slice2))
	copy(sorted1, slice1)
	copy(sorted2, slice2)

	sort.Strings(sorted1)
	sort.Strings(sorted2)

	for i := range sorted1 {
		if sorted1[i] != sorted2[i] {
			return false
		}
	}
	return true
}

// difference returns elements in source that are not in exclude.
// Returns a new slice containing only elements unique to source.
// Always returns a non-nil slice.
func difference(source, exclude []string) []string {
	diff := make([]string, 0)

	if len(source) == 0 {
		return diff
	}

	excludeSet := make(map[string]bool, len(exclude))
	for _, item := range exclude {
		excludeSet[item] = true
	}

	for _, item := range source {
		if !excludeSet[item] {
			diff = append(diff, item)
		}
	}

	return diff
}
