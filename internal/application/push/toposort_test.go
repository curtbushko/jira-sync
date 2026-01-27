package push

import (
	"testing"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopologicalSort_SimpleChain(t *testing.T) {
	// KB-1 -> KB-2 -> KB-3
	tasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-3: Third", JiraDependencies: []string{"KB-2"}, JiraParent: "P"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-1: First", JiraDependencies: []string{}, JiraParent: "P"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Second", JiraDependencies: []string{"KB-1"}, JiraParent: "P"}},
	}

	sorted, err := TopologicalSort(tasks, tasks)

	require.NoError(t, err)
	require.Len(t, sorted, 3)
	assert.Equal(t, "KB-1", sorted[0].TaskID())
	assert.Equal(t, "KB-2", sorted[1].TaskID())
	assert.Equal(t, "KB-3", sorted[2].TaskID())
}

func TestTopologicalSort_MultipleDependencies(t *testing.T) {
	// CTRL-1 depends on both KB-3 and ERR-1
	tasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "CTRL-1: Controller", JiraDependencies: []string{"KB-3", "ERR-1"}, JiraParent: "P"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-3: Types", JiraDependencies: []string{}, JiraParent: "P"}},
		{Frontmatter: domain.Frontmatter{Title: "ERR-1: Detector", JiraDependencies: []string{}, JiraParent: "P"}},
	}

	sorted, err := TopologicalSort(tasks, tasks)

	require.NoError(t, err)
	require.Len(t, sorted, 3)
	// CTRL-1 must be last (depends on both KB-3 and ERR-1)
	assert.Equal(t, "CTRL-1", sorted[2].TaskID())
	// KB-3 and ERR-1 can be in either order (both have no deps)
}

func TestTopologicalSort_CircularDependency(t *testing.T) {
	// KB-1 -> KB-2 -> KB-1 (circular!)
	tasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-1: First", JiraDependencies: []string{"KB-2"}, JiraParent: "P"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Second", JiraDependencies: []string{"KB-1"}, JiraParent: "P"}},
	}

	_, err := TopologicalSort(tasks, tasks)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular")
}

func TestTopologicalSort_DependencyAlreadyCreated(t *testing.T) {
	// KB-2 depends on KB-1, but KB-1 is already created (not in pending list)
	pending := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Second", JiraDependencies: []string{"KB-1"}, JiraParent: "P"}},
	}
	allTasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-1: First", SyncStatus: domain.SyncStatusCreated, JiraNumber: "GUARD-101", JiraParent: "P"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Second", JiraDependencies: []string{"KB-1"}, JiraParent: "P"}},
	}

	sorted, err := TopologicalSort(pending, allTasks)

	require.NoError(t, err)
	require.Len(t, sorted, 1)
	assert.Equal(t, "KB-2", sorted[0].TaskID())
}

func TestTopologicalSort_NoDependencies(t *testing.T) {
	// No dependencies - any order is valid
	tasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-1: First", JiraDependencies: []string{}, JiraParent: "P"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Second", JiraDependencies: []string{}, JiraParent: "P"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-3: Third", JiraDependencies: []string{}, JiraParent: "P"}},
	}

	sorted, err := TopologicalSort(tasks, tasks)

	require.NoError(t, err)
	require.Len(t, sorted, 3)
	// All tasks should be present (order doesn't matter)
	ids := make(map[string]bool)
	for _, task := range sorted {
		ids[task.TaskID()] = true
	}
	assert.True(t, ids["KB-1"])
	assert.True(t, ids["KB-2"])
	assert.True(t, ids["KB-3"])
}

func TestTopologicalSort_WikiLinkFormat(t *testing.T) {
	// Dependencies using wiki link format
	tasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Second", JiraDependencies: []string{"[KB-1: First](20260116.md)"}, JiraParent: "P"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-1: First", JiraDependencies: []string{}, JiraParent: "P"}},
	}

	sorted, err := TopologicalSort(tasks, tasks)

	require.NoError(t, err)
	require.Len(t, sorted, 2)
	assert.Equal(t, "KB-1", sorted[0].TaskID())
	assert.Equal(t, "KB-2", sorted[1].TaskID())
}

func TestTopologicalSort_EmptyList(t *testing.T) {
	sorted, err := TopologicalSort([]*domain.TaskFile{}, []*domain.TaskFile{})

	require.NoError(t, err)
	assert.Len(t, sorted, 0)
}

func TestTopologicalSort_DiamondDependency(t *testing.T) {
	// Diamond: KB-4 depends on KB-2 and KB-3, both depend on KB-1
	//     KB-1
	//    /    \
	//  KB-2   KB-3
	//    \    /
	//     KB-4
	tasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-4: Final", JiraDependencies: []string{"KB-2", "KB-3"}, JiraParent: "P"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Branch A", JiraDependencies: []string{"KB-1"}, JiraParent: "P"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-3: Branch B", JiraDependencies: []string{"KB-1"}, JiraParent: "P"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-1: Root", JiraDependencies: []string{}, JiraParent: "P"}},
	}

	sorted, err := TopologicalSort(tasks, tasks)

	require.NoError(t, err)
	require.Len(t, sorted, 4)
	// KB-1 must be first
	assert.Equal(t, "KB-1", sorted[0].TaskID())
	// KB-4 must be last
	assert.Equal(t, "KB-4", sorted[3].TaskID())
	// KB-2 and KB-3 must come before KB-4
	ids := []string{sorted[1].TaskID(), sorted[2].TaskID()}
	assert.Contains(t, ids, "KB-2")
	assert.Contains(t, ids, "KB-3")
}

func TestTopologicalSort_ExternalJiraDependency(t *testing.T) {
	// KB-2 depends on GUARD-999, which is an external Jira issue (not in local tasks)
	// This should NOT be an error - external dependencies are allowed
	tasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Second", JiraDependencies: []string{"GUARD-999"}, JiraParent: "P"}},
	}

	sorted, err := TopologicalSort(tasks, tasks)

	require.NoError(t, err)
	assert.Len(t, sorted, 1)
	assert.Equal(t, "KB-2", sorted[0].TaskID())
}
