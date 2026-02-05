---
title: Jira Ticket Management Plan
date: 2026-01-16 10:30
tags:
  - jira
  - planning
  - tooling
---

# Jira Ticket Management Plan

## Overview

`jira-sync` is a Go CLI tool for managing Jira tickets through local markdown files. It enables bidirectional synchronization between local task files and Jira issues.

## Architecture (Hexagonal / Ports & Adapters)

The codebase follows **Hexagonal Architecture** (Ports & Adapters pattern), ensuring business logic is isolated from external dependencies.

```
┌─────────────────────────────────────────────────────────────────┐
│                         CLI (cmd/)                              │
│   root.go, create.go, export.go, push.go, pull.go, migrate.go  │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Application Services                          │
│              (internal/application/)                            │
│   ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌────────────┐        │
│   │  push/  │  │  pull/  │  │ export/ │  │ transition/│        │
│   └────┬────┘  └────┬────┘  └────┬────┘  └─────┬──────┘        │
└────────┼────────────┼───────────┼──────────────┼────────────────┘
         │            │           │              │
         ▼            ▼           ▼              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Ports (Interfaces)                           │
│                   (internal/ports/)                             │
│   TaskRepository │ JiraClient │ HashComputer │ UserPrompter    │
└────────┬────────────┬───────────┬──────────────┬────────────────┘
         │            │           │              │
         ▼            ▼           ▼              ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Adapters (Implementations)                    │
│                   (internal/adapters/)                          │
│   ┌────────────┐  ┌─────────┐  ┌──────────┐                    │
│   │ filesystem/│  │  jira/  │  │ hashing/ │                    │
│   │  parser    │  │ client  │  │  sha256  │                    │
│   │  writer    │  │         │  │          │                    │
│   │ repository │  │         │  │          │                    │
│   └────────────┘  └─────────┘  └──────────┘                    │
└─────────────────────────────────────────────────────────────────┘
         │            │           │
         ▼            ▼           ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Domain (Core)                              │
│                   (internal/domain/)                            │
│   TaskFile │ Frontmatter │ Constants │ Validation │ Errors     │
└─────────────────────────────────────────────────────────────────┘
```

### Directory Structure

```
jira-sync/
├── cmd/                          # CLI entry points (Cobra commands)
│   ├── root.go                   # Root command + Viper config
│   ├── create.go                 # create - generate task files
│   ├── export.go                 # export - import from Jira
│   ├── push.go                   # push - local → Jira
│   ├── pull.go                   # pull - Jira → local
│   └── migrate.go                # migrate - update legacy files
│
├── internal/
│   ├── domain/                   # Core business types (no dependencies)
│   │   ├── task.go               # TaskFile, Frontmatter structs
│   │   ├── constants.go          # SyncStatus, defaults
│   │   ├── validation.go         # Field validation
│   │   └── errors.go             # Domain errors
│   │
│   ├── ports/                    # Interface definitions
│   │   └── ports.go              # TaskRepository, JiraClient, HashComputer
│   │
│   ├── adapters/                 # Port implementations
│   │   ├── filesystem/           # TaskRepository implementation
│   │   │   ├── parser.go         # Parse markdown + YAML frontmatter
│   │   │   ├── writer.go         # Write task files
│   │   │   ├── repository.go     # ListTasks, ReadTask, WriteTask
│   │   │   └── wikilink.go       # Wiki link parsing
│   │   ├── jira/                 # JiraClient implementation
│   │   │   ├── client.go         # Jira REST API client
│   │   │   └── mock_client.go    # Mock for testing
│   │   └── hashing/              # HashComputer implementation
│   │       └── sha256.go         # SHA256 content hashing
│   │
│   ├── application/              # Business logic services
│   │   ├── push/                 # Push service (local → Jira)
│   │   │   ├── service.go        # CreateTickets, LinkDependencies
│   │   │   └── toposort.go       # Topological sort for dependencies
│   │   ├── pull/                 # Pull service (Jira → local)
│   │   │   ├── service.go        # PullTask, PullAll
│   │   │   └── detector.go       # Change detection
│   │   ├── export/               # Export service
│   │   │   └── service.go        # Export Jira issue to local
│   │   └── transition/           # Workflow transitions
│   │       └── service.go        # Status transitions
│   │
│   ├── config/                   # Configuration (Viper)
│   │   └── config.go             # Load and validate config
│   │
│   └── integration/              # Integration tests
│       └── integration_test.go
│
├── main.go                       # Entry point
├── go.mod
└── go.sum
```

### Key Interfaces (Ports)

```go
// TaskRepository - file system abstraction
type TaskRepository interface {
    ReadTask(path string) (*domain.TaskFile, error)
    WriteTask(task *domain.TaskFile) error
    ListTasks(dir string) ([]*domain.TaskFile, error)
    GenerateFilename() string
}

// JiraClient - Jira API abstraction
type JiraClient interface {
    CreateIssue(ctx, req) (*Issue, error)
    UpdateIssue(ctx, key, req) error
    GetIssue(ctx, key) (*Issue, error)
    GetIssueWithLinks(ctx, key) (*IssueWithLinks, error)
    CreateLink(ctx, inward, outward, linkType) error
    DeleteLink(ctx, linkID) error
    GetIssueLinks(ctx, key) ([]IssueLink, error)
    GetTransitions(ctx, key) ([]Transition, error)
    DoTransition(ctx, key, transitionID) error
    BaseURL() string
}

// HashComputer - content hashing abstraction
type HashComputer interface {
    ComputeHash(task *domain.TaskFile) string
}
```

---

## Task File Format

Each task is stored as a markdown file with YAML frontmatter:

```markdown
---
title: "KB-1: Kubebuilder - Initialize Project"
jira-number: ""
jira-project: GUARD
jira-type: Story
jira-state: Todo
created-date: 2026-01-16
jira-url: ""
sync-status: pending
jira-parent: GUARD-100
sync-dependencies: []
jira-dependencies: []
content-hash: ""
---

Initialize the Kubebuilder project using the CLI tool.

## Acceptance Criteria

- Kubebuilder project initialized
- Go module created
- Basic Makefile exists
```

### Frontmatter Fields

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | Task ID and title (e.g., "KB-1: Initialize Project") |
| `jira-number` | string | Jira issue key (e.g., "GUARD-123") - populated after push |
| `jira-project` | string | Jira project key (e.g., "GUARD") |
| `jira-type` | string | Issue type: "Story", "Task", "Bug", "Epic" (default: "Story") |
| `jira-state` | string | Jira status: "Todo", "In Progress", "Done" (default: "Todo") |
| `created-date` | date | Date the local file was created |
| `jira-url` | string | Full URL to Jira issue - populated after push |
| `sync-status` | string | Sync state: "pending", "created", "linked" |
| `jira-parent` | string | Parent epic/story key (not required for Epics) |
| `sync-dependencies` | array | Task IDs for creation ordering (local only) |
| `jira-dependencies` | array | Task IDs for Jira "blocks" links |
| `content-hash` | string | SHA256 hash for change detection |

### Sync Status Values

| Status | Meaning |
|--------|---------|
| `pending` | Not yet created in Jira |
| `created` | Created in Jira, dependencies not yet linked |
| `linked` | Fully synced with Jira |

### Dependency Types

**`sync-dependencies`** - Local creation ordering only:
- Determines order in which tickets are created during `push`
- Uses topological sort to ensure dependencies are created first
- Does NOT create links in Jira

**`jira-dependencies`** - Jira "blocks" links:
- Creates "blocks" links between Jira tickets
- Does NOT affect creation order
- Bidirectional: updated during both `push` and `pull`

**Wiki Link Format:**
```yaml
jira-dependencies:
  - "[KB-1: Initialize Project](20260116-103001.md)"
```

---

## CLI Commands

### 1. create - Generate Task Files

Create a new markdown task file with proper frontmatter.

```bash
jira-sync create [flags]
```

**Flags:**

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--title` | `-t` | Yes | Task ID and title |
| `--description` | `-d` | Yes | Task description (becomes Jira description) |
| `--jira-parent` | `-p` | No* | Parent epic/story key (*required except for Epics) |
| `--jira-project` | | No | Jira project key |
| `--type` | | No | Issue type (default: "Story") |
| `--deps` | | No | Sets both sync-deps and jira-deps |
| `--sync-deps` | | No | Creation ordering dependencies |
| `--jira-deps` | | No | Jira "blocks" link dependencies |
| `--output` | `-o` | No | Output directory (default: ".") |

**Examples:**

```bash
# Simple task
jira-sync create \
  --title "KB-1: Initialize Project" \
  --jira-parent GUARD-100 \
  --description "Initialize the project"

# Task with dependencies
jira-sync create \
  --title "KB-2: Create Types" \
  --jira-parent GUARD-100 \
  --deps "KB-1" \
  --description "Create shared types"

# Epic (no parent required)
jira-sync create \
  --title "GUARDIAN: Error Operator Epic" \
  --type Epic \
  --description "Epic for the error operator"
```

**Output:**
- Creates file: `./20260116-103001.md`
- Sets `sync-status: pending`

---

### 2. export - Import Jira Issue to Local

Export an existing Jira issue to a local markdown task file.

```bash
jira-sync export <jira-id> [flags]
```

**Arguments:**
- `jira-id` - Jira issue key (e.g., GUARD-123)

**Flags:**

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--output` | `-o` | No | Output directory (default: ".") |
| `--parent` | `-p` | No | Override jira-parent value |
| `--force` | `-f` | No | Overwrite existing file |

**Examples:**

```bash
# Export a Jira issue
jira-sync export GUARD-123

# Export with custom output directory
jira-sync export GUARD-123 --output ./tasks/

# Override parent and force overwrite
jira-sync export GUARD-123 --parent GUARD-100 --force
```

**Behavior:**
- Fetches issue from Jira with all fields
- Extracts blocking dependencies from issue links
- Maps Jira keys to local task IDs if possible
- Generates zettelkasten filename from creation date
- Sets `sync-status: linked`
- Computes `content-hash`

---

### 3. push - Local to Jira

Push local task file changes to Jira.

```bash
jira-sync push [tasks-dir] [flags]
```

**Arguments:**
- `tasks-dir` - Directory containing task files (default: ".")

**Flags:**

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--project` | `-p` | No | Default Jira project key |
| `--dry-run` | | No | Show what would happen |
| `--yes` | `-y` | No | Skip confirmation prompts |
| `--create-only` | | No | Only create tickets |
| `--link-only` | | No | Only link dependencies |
| `--status-only` | | No | Only show status |

**Examples:**

```bash
# Push all changes
jira-sync push

# Push with specific directory
jira-sync push ./tasks/ --project GUARD

# Dry run
jira-sync push --dry-run

# Skip confirmation
jira-sync push --yes
```

**Phases:**

1. **Scan & Categorize:**
   - `pending` - Will create tickets
   - `created` - Will link dependencies
   - `linked` (modified) - Will update description
   - `linked` (unchanged) - Up to date

2. **Create Phase:**
   - Topologically sorts pending tasks by `sync-dependencies`
   - Creates Jira tickets in order
   - Updates local files with `jira-number` and `jira-url`
   - Sets `sync-status: created`

3. **Link Phase:**
   - Creates "blocks" links based on `jira-dependencies`
   - Sets `sync-status: linked`
   - Computes and stores `content-hash`

4. **Update Phase:**
   - Detects modified files via `content-hash`
   - Updates Jira description and title
   - Recomputes `content-hash`

---

### 4. pull - Jira to Local

Pull changes from Jira to local task files.

```bash
jira-sync pull [tasks-dir] [flags]
```

**Arguments:**
- `tasks-dir` - Directory containing task files (default: ".")

**Flags:**

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--dry-run` | | No | Show what would happen |
| `--yes` | `-y` | No | Skip confirmation prompts |
| `--force` | | No | Overwrite local changes (resolve conflicts) |

**Examples:**

```bash
# Pull all updates
jira-sync pull

# Pull with dry run
jira-sync pull --dry-run

# Force overwrite local changes
jira-sync pull --force
```

**Behavior:**

1. **Filter:** Only processes tasks with `jira-number` set

2. **Fetch & Compare:** For each task:
   - Fetches current Jira ticket
   - Compares local vs Jira fields

3. **Fields Updated:**
   - `jira-state` ← Jira status
   - `title` ← Jira summary
   - `description` ← Jira description
   - `jira-dependencies` ← Jira "blocks" links

4. **Conflict Detection:**
   - Local changed: `content-hash` differs from file
   - Both changed: CONFLICT (use `--force` to resolve)

5. **Dependency Mapping:**
   - Extracts Jira "blocks" links
   - Maps Jira keys to local task IDs
   - Updates `jira-dependencies` array

**Output Actions:**
- `updated` - Applied Jira changes to local
- `skipped` - No changes needed
- `conflict` - Both sides changed (use `--force`)
- `error` - Failed to fetch or update

---

### 5. migrate - Update Legacy Files

Add missing frontmatter fields to older task files.

```bash
jira-sync migrate [tasks-dir] [flags]
```

**Arguments:**
- `tasks-dir` - Directory containing task files (default: ".")

**Flags:**

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--dry-run` | | No | Show what would be migrated |
| `--default-project` | | No | Default `jira-project` for tasks missing it |

**Examples:**

```bash
# Migrate all task files
jira-sync migrate

# Dry run
jira-sync migrate --dry-run

# Set default project
jira-sync migrate --default-project GUARD
```

**Missing Field Defaults:**

| Field | Default |
|-------|---------|
| `jira-number` | `""` |
| `jira-project` | `""` (use `--default-project`) |
| `jira-type` | `"Story"` |
| `jira-state` | `"Todo"` |
| `jira-url` | `""` |
| `sync-status` | `"pending"` |
| `sync-dependencies` | `[]` |
| `jira-dependencies` | `[]` |
| `content-hash` | `""` |

---

## Configuration

### Config File (~/.jira-sync.yaml)

```yaml
jira:
  url: "https://company.atlassian.net"
  user: "user@company.com"

defaults:
  issue_type: "Story"
  project: "GUARD"

link_types:
  dependency: "Blocks"
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `JIRA_URL` | Jira instance URL |
| `JIRA_USER` | Jira username/email |
| `JIRA_TOKEN` | Jira API token (required, never in config file) |

### Precedence

1. CLI flags (highest)
2. Environment variables
3. Config file
4. Built-in defaults (lowest)

---

## Workflows

### Creating New Tasks

```bash
# 1. Create task files
jira-sync create --title "KB-1: Init Project" --jira-parent GUARD-100 -d "..."
jira-sync create --title "KB-2: Create Types" --jira-parent GUARD-100 --deps "KB-1" -d "..."

# 2. Review generated files
ls *.md

# 3. Push to Jira
jira-sync push --dry-run
jira-sync push
```

### Syncing Existing Tasks

```bash
# Pull latest from Jira
jira-sync pull

# Make local changes (edit .md files)
vim 20260116-103001.md

# Push changes back
jira-sync push
```

### Importing Existing Jira Tickets

```bash
# Export single ticket
jira-sync export GUARD-123

# Export to specific directory
jira-sync export GUARD-123 --output ./tasks/
```

### Migrating Legacy Files

```bash
# Check what needs migration
jira-sync migrate --dry-run

# Migrate with default project
jira-sync migrate --default-project GUARD
```

---

## Change Detection

The `content-hash` field stores a SHA256 hash of the task file (frontmatter + description, excluding hash itself).

**Push triggers:**
- Hash differs from stored `content-hash` → update Jira

**Pull conflict detection:**
- Local hash differs → local changed
- Both differ → CONFLICT

| sync-status | content-hash | Action |
|-------------|--------------|--------|
| `pending` | empty | Create in Jira |
| `created` | empty | Link dependencies |
| `linked` | matches | No action |
| `linked` | differs | Update Jira |

---

## Error Handling

### Validation Before Push

- All `sync-dependencies` must exist as task files
- No circular dependencies (topological sort fails)
- `jira-project` must be set for pending tasks
- `jira-parent` required for non-Epic types

### Rollback Strategy

If creation fails mid-batch:
- Already created tickets remain in Jira
- Local files updated with `jira-number` are preserved
- Re-running `push` continues from where it stopped

---

## Security Notes

- `JIRA_TOKEN` must be set via environment variable only
- Never commit API tokens to config files
- Generate tokens at: https://id.atlassian.com/manage-profile/security/api-tokens

