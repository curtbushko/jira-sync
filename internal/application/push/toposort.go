package push

import (
	"fmt"

	"github.com/curtbushko/jira-sync/internal/domain"
)

// TopologicalSort orders tasks so that dependencies come before dependent tasks.
// It uses jira-is-blocked-by to determine ordering (tasks we depend on must be created first).
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

	taskByID := buildTaskIndex(allTasks)
	pendingSet := buildPendingSet(pending)
	adjList, inDegree, err := buildDependencyGraph(pending, taskByID, pendingSet)
	if err != nil {
		return nil, err
	}

	return runKahnsAlgorithm(pending, taskByID, adjList, inDegree)
}

// buildTaskIndex creates a map from task ID and Jira key to task.
func buildTaskIndex(allTasks []*domain.TaskFile) map[string]*domain.TaskFile {
	taskByID := make(map[string]*domain.TaskFile, len(allTasks)*2)
	for _, task := range allTasks {
		taskByID[task.TaskID()] = task
		if task.Frontmatter.JiraNumber != "" {
			taskByID[task.Frontmatter.JiraNumber] = task
		}
	}
	return taskByID
}

// buildPendingSet creates a set of pending task IDs for quick lookup.
func buildPendingSet(pending []*domain.TaskFile) map[string]bool {
	pendingSet := make(map[string]bool, len(pending))
	for _, task := range pending {
		pendingSet[task.TaskID()] = true
	}
	return pendingSet
}

// buildDependencyGraph builds the adjacency list and in-degree map for topological sort.
func buildDependencyGraph(
	pending []*domain.TaskFile,
	taskByID map[string]*domain.TaskFile,
	pendingSet map[string]bool,
) (map[string][]string, map[string]int, error) {
	adjList := make(map[string][]string)
	inDegree := make(map[string]int)

	for _, task := range pending {
		taskID := task.TaskID()
		adjList[taskID] = []string{}
		inDegree[taskID] = 0
	}

	for _, task := range pending {
		if err := processDependencies(task, taskByID, pendingSet, adjList, inDegree); err != nil {
			return nil, nil, err
		}
	}

	return adjList, inDegree, nil
}

// processDependencies processes the dependencies for a single task.
func processDependencies(
	task *domain.TaskFile,
	taskByID map[string]*domain.TaskFile,
	pendingSet map[string]bool,
	adjList map[string][]string,
	inDegree map[string]int,
) error {
	taskID := task.TaskID()

	for _, depID := range task.Frontmatter.JiraIsBlockedBy {
		resolvedDepID := parseWikiLink(depID)
		depTask, exists := taskByID[resolvedDepID]
		if !exists {
			// External Jira issue - skip validation
			continue
		}

		depTaskID := depTask.TaskID()
		if pendingSet[depTaskID] {
			adjList[depTaskID] = append(adjList[depTaskID], taskID)
			inDegree[taskID]++
		} else if depTask.Frontmatter.SyncStatus != domain.SyncStatusCreated &&
			depTask.Frontmatter.SyncStatus != domain.SyncStatusLinked {
			return fmt.Errorf("dependency not found in pending tasks: %s required by %s", depID, taskID)
		}
	}

	return nil
}

// runKahnsAlgorithm performs Kahn's algorithm for topological sort.
func runKahnsAlgorithm(
	pending []*domain.TaskFile,
	taskByID map[string]*domain.TaskFile,
	adjList map[string][]string,
	inDegree map[string]int,
) ([]*domain.TaskFile, error) {
	var queue []string
	for taskID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, taskID)
		}
	}

	var sorted []*domain.TaskFile
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		sorted = append(sorted, taskByID[current])

		for _, dependent := range adjList[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(sorted) != len(pending) {
		return nil, detectCyclicDependency(inDegree)
	}

	return sorted, nil
}

// detectCyclicDependency returns an error describing the cyclic dependency.
func detectCyclicDependency(inDegree map[string]int) error {
	var cycleNodes []string
	for taskID, degree := range inDegree {
		if degree > 0 {
			cycleNodes = append(cycleNodes, taskID)
		}
	}
	return fmt.Errorf("circular dependency detected involving: %v", cycleNodes)
}
