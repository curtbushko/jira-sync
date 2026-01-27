package push

import (
	"fmt"

	"github.com/curtbushko/jira-sync/internal/domain"
)

// TopologicalSort orders tasks so that dependencies come before dependent tasks.
// It uses sync-dependencies to determine ordering (not jira-dependencies).
//
// Parameters:
//   - pending: tasks to sort (typically tasks with sync-status: pending)
//   - allTasks: all tasks (including already-created ones) for dependency lookup
//
// Returns:
//   - sorted list of tasks where dependencies come first
//   - error if circular dependency is detected or dependency not found
func TopologicalSort(pending, allTasks []*domain.TaskFile) ([]*domain.TaskFile, error) {
	if len(pending) == 0 {
		return []*domain.TaskFile{}, nil
	}

	// Build task ID to task map for all tasks
	taskByID := make(map[string]*domain.TaskFile, len(allTasks))
	for _, task := range allTasks {
		taskByID[task.TaskID()] = task
	}

	// Build pending set for quick lookup
	pendingSet := make(map[string]bool, len(pending))
	for _, task := range pending {
		pendingSet[task.TaskID()] = true
	}

	// Build adjacency list for pending tasks only
	// Each task maps to its dependencies that are also pending
	adjList := make(map[string][]string)
	inDegree := make(map[string]int)

	for _, task := range pending {
		taskID := task.TaskID()
		adjList[taskID] = []string{}
		inDegree[taskID] = 0
	}

	// Process dependencies
	for _, task := range pending {
		taskID := task.TaskID()
		depIDs := task.GetSyncDependencyIDs()

		for _, depID := range depIDs {
			// Check if dependency exists
			depTask, exists := taskByID[depID]
			if !exists {
				return nil, fmt.Errorf("dependency not found: %s required by %s", depID, taskID)
			}

			// Only count dependencies that are in the pending set
			// (already created tasks don't affect ordering)
			if pendingSet[depID] {
				// depID -> taskID (depID must come before taskID)
				adjList[depID] = append(adjList[depID], taskID)
				inDegree[taskID]++
			} else if depTask.Frontmatter.SyncStatus != domain.SyncStatusCreated &&
				depTask.Frontmatter.SyncStatus != domain.SyncStatusLinked {
				// Dependency exists but is not yet created - should be in pending
				return nil, fmt.Errorf("dependency not found in pending tasks: %s required by %s", depID, taskID)
			}
		}
	}

	// Kahn's algorithm for topological sort
	var queue []string
	for taskID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, taskID)
		}
	}

	var sorted []*domain.TaskFile
	for len(queue) > 0 {
		// Pop from queue
		current := queue[0]
		queue = queue[1:]

		// Add to sorted result
		sorted = append(sorted, taskByID[current])

		// Reduce in-degree for all dependent tasks
		for _, dependent := range adjList[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	// Check for circular dependency
	if len(sorted) != len(pending) {
		// Find tasks involved in cycle
		var cycleNodes []string
		for taskID, degree := range inDegree {
			if degree > 0 {
				cycleNodes = append(cycleNodes, taskID)
			}
		}
		return nil, fmt.Errorf("circular dependency detected involving: %v", cycleNodes)
	}

	return sorted, nil
}
