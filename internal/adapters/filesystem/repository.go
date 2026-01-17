package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/curtbushko/jira-sync/internal/domain"
)

// FileTaskRepository implements ports.TaskRepository using the local file system.
type FileTaskRepository struct {
	parser *Parser
	writer *Writer
}

// NewFileTaskRepository creates a new FileTaskRepository.
func NewFileTaskRepository() *FileTaskRepository {
	return &FileTaskRepository{
		parser: NewParser(),
		writer: NewWriter(),
	}
}

// ReadTask reads a single task file and parses frontmatter.
func (r *FileTaskRepository) ReadTask(path string) (*domain.TaskFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}

	task, err := r.parser.Parse(path, string(content))
	if err != nil {
		return nil, err
	}

	return task, nil
}

// WriteTask writes a task file with frontmatter and description.
func (r *FileTaskRepository) WriteTask(task *domain.TaskFile) error {
	// Ensure directory exists
	dir := filepath.Dir(task.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	content, err := r.writer.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}

	if err := os.WriteFile(task.Path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write file %s: %w", task.Path, err)
	}

	return nil
}

// ListTasks returns all task files in a directory.
func (r *FileTaskRepository) ListTasks(dir string) ([]*domain.TaskFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read directory %s: %w", dir, err)
	}

	var tasks []*domain.TaskFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		task, err := r.ReadTask(path)
		if err != nil {
			return nil, fmt.Errorf("read task %s: %w", path, err)
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}

// GenerateFilename creates a zettelkasten filename (YYYYMMDD-HHMMSS.md).
func (r *FileTaskRepository) GenerateFilename() string {
	return time.Now().Format("20060102-150405") + ".md"
}
