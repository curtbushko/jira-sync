# jira-sync

A Go CLI tool for managing Jira tickets from local markdown files.

## Features

- **Batch ticket creation**: Create multiple Jira tickets from markdown files
- **Dependency linking**: Automatically link dependencies between tickets
- **Topological ordering**: Create tickets in correct dependency order
- **Two-way sync**: Keep local files in sync with Jira
- **Change detection**: SHA256 hashing to detect modified content
- **Zettelkasten naming**: Files use timestamp-based names (YYYYMMDD-HHMMSS.md)

## Installation

```bash
go install github.com/curtbushko/jira-sync@latest
```

Or build from source:

```bash
git clone https://github.com/curtbushko/jira-sync.git
cd jira-sync
go build -o jira-sync .
```

## Configuration

### Environment Variables

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `JIRA_TOKEN` | **Yes** | Jira API token | `ATATT3xFfGF0abc123...` |
| `JIRA_URL` | Yes* | Jira instance URL | `https://company.atlassian.net` |
| `JIRA_USER` | Yes* | Jira username/email | `user@company.com` |
| `JIRA_DEFAULTS_PROJECT` | No | Default project key | `MYPROJ` |
| `JIRA_DEFAULTS_ISSUE_TYPE` | No | Default issue type (default: `Task`) | `Story`, `Bug`, `Task` |
| `JIRA_DEFAULTS_END_DATE_OFFSET` | No | Days to add for end date (default: `7`) | `14` |
| `JIRA_LINK_TYPES_DEPENDENCY` | No | Link type name (default: `Blocks`) | `Blocks`, `Relates` |

*Can also be set in config file

#### Example Setup

```bash
# Required
export JIRA_TOKEN="ATATT3xFfGF0abc123def456..."
export JIRA_URL="https://mycompany.atlassian.net"
export JIRA_USER="developer@mycompany.com"

# Optional defaults
export JIRA_DEFAULTS_PROJECT="MYPROJ"
export JIRA_DEFAULTS_ISSUE_TYPE="Task"
```

### Config File (~/.jira-sync.yaml)

```yaml
jira:
  url: https://company.atlassian.net
  user: user@company.com
  # NOTE: Never put token in config file - use JIRA_TOKEN env var

defaults:
  project: GUARD
  issue_type: Task
  end_date_offset: 7

link_types:
  dependency: Blocks
```

### Generating a Jira API Token

1. Log in to https://id.atlassian.com/manage-profile/security/api-tokens
2. Click "Create API token"
3. Give it a descriptive label (e.g., "jira-sync CLI")
4. Copy the token and set it as `JIRA_TOKEN`

```bash
export JIRA_TOKEN="ATATT3xFfGF0..."
```

## Usage

### Create a Task File

```bash
# Simple task with no dependencies
jira-sync create \
  --title "KB-1: Initialize Project" \
  --parent GUARD-100 \
  --description "Initialize the kubebuilder project"

# Task with dependencies (shorthand sets both sync and jira deps)
jira-sync create \
  --title "CTRL-1: Controller Scaffold" \
  --parent GUARD-100 \
  --description "Create the basic controller structure." \
  --deps "KB-3,ERR-1"

# Task with different sync and jira dependencies
jira-sync create \
  --title "ERR-5: Implement Pod Listing" \
  --parent GUARD-100 \
  --description "List pods by deployment." \
  --sync-deps "KB-3" \
  --jira-deps "KB-3,ERR-1"

# Multi-line description with heredoc
jira-sync create \
  --title "ERR-2: Replica Detection" \
  --parent GUARD-100 \
  --deps "ERR-1" \
  --description "$(cat <<'EOF'
Implement detection of replica failures.

## Acceptance Criteria

- Check deployment.Status.UnavailableReplicas
- Create DeploymentError when count > 0
- Unit tests required
EOF
)"
```

### Sync with Jira

```bash
# Set credentials
export JIRA_TOKEN="your-api-token"
export JIRA_URL="https://company.atlassian.net"
export JIRA_USER="user@company.com"

# Full sync (create tickets + link dependencies)
jira-sync sync ./tasks/ --project GUARD

# Dry run first to see what will happen
jira-sync sync ./tasks/ --project GUARD --dry-run

# Skip confirmation prompts (for scripting)
jira-sync sync ./tasks/ --project GUARD --yes

# Only create tickets (don't link dependencies yet)
jira-sync sync ./tasks/ --project GUARD --create-only

# Only link dependencies (tickets already created)
jira-sync sync ./tasks/ --project GUARD --link-only
```

---

## Task File Format

Each task is a markdown file with YAML frontmatter. Files use zettelkasten naming: `YYYYMMDD-HHMMSS.md`

### Template

```markdown
---
title: "TASK-ID: Task Title"
jira-number: ""
created-date: 2026-01-16
start-date: ""
end-date: ""
jira-url: ""
sync-status: pending
parent: PARENT-KEY
sync-dependencies: []
jira-dependencies: []
content-hash: ""
---

Task description goes here. This becomes the Jira ticket description.

## Acceptance Criteria

- First criterion
- Second criterion
- Third criterion
```

### Example (After Sync)

```markdown
---
title: "KB-1: Kubebuilder - Initialize Project and Repository"
jira-number: "GUARD-101"
created-date: 2026-01-16
start-date: 2026-01-16
end-date: 2026-01-23
jira-url: "https://company.atlassian.net/browse/GUARD-101"
sync-status: linked
parent: GUARD-100
sync-dependencies: []
jira-dependencies: []
content-hash: "a1b2c3d4e5f6789..."
---

Initialize the Kubebuilder project using the CLI tool.

## Acceptance Criteria

- Kubebuilder project initialized
- Go module created with go.mod
- Repository pushed to remote
```

---

## Frontmatter Fields Reference

### All Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `title` | string | **Yes** | Task ID and title (e.g., "KB-1: Initialize Project") |
| `project` | string | No | Jira project key (overrides default, e.g., "MYPROJ") |
| `jira-number` | string | No | Jira issue key (auto-set after creation, e.g., "GUARD-101") |
| `created-date` | date | No | Date file was created (auto-set by `create`) |
| `start-date` | date | No | Date ticket created in Jira (auto-set by `sync`) |
| `end-date` | date | No | Start date + offset days (auto-set by `sync`) |
| `jira-url` | string | No | Full URL to Jira issue (auto-set by `sync`) |
| `sync-status` | string | **Yes** | Sync state: `pending`, `created`, `linked` |
| `parent` | string | **Yes** | Parent epic/story key (e.g., "GUARD-100") |
| `sync-dependencies` | array | No | Task IDs for creation ordering |
| `jira-dependencies` | array | No | Task IDs for Jira "blocks" links |
| `content-hash` | string | No | SHA256 hash for change detection (auto-set by `sync`) |

### Field Details

#### `title`
Format: `"TASK-ID: Human Readable Title"`

The task ID prefix (before the colon) is used to:
- Reference this task in dependencies
- Create readable Jira ticket summaries

```yaml
title: "KB-1: Kubebuilder - Initialize Project and Repository"
#       ^^^^  ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
#       ID    Human readable title
```

#### `project`

Optional Jira project key for this task. If set, overrides the default project from `--project` flag or `JIRA_DEFAULTS_PROJECT` environment variable.

```yaml
project: "BACKEND"  # This task goes to BACKEND project instead of the default
```

Useful when tasks in the same directory need to go to different Jira projects.

#### `sync-status`

| Value | Meaning | Next Action |
|-------|---------|-------------|
| `pending` | Not yet created in Jira | Will create ticket |
| `created` | Ticket exists, links not created | Will create Jira links |
| `linked` | Fully synced | Check for content changes |

#### `sync-dependencies`

**Purpose**: Controls the ORDER in which tickets are created.

- Tasks listed here must be created in Jira BEFORE this task
- Uses topological sort to determine creation order
- Does NOT create any links in Jira
- Circular dependencies cause an error

```yaml
sync-dependencies:
  - KB-1    # KB-1's ticket must be created before this task's ticket
  - KB-2
```

#### `jira-dependencies`

**Purpose**: Creates "blocks" links in Jira.

- Creates "X blocks Y" links in Jira after tickets are created
- Does NOT affect creation order
- Referenced tasks must exist (either already created or in the batch)

```yaml
jira-dependencies:
  - KB-1    # Will create: "GUARD-101 blocks GUARD-XXX" link in Jira
  - ERR-1
```

#### `content-hash`

SHA256 hash of: `title + parent + jira-dependencies + description`

Used to detect changes that need re-syncing to Jira. Note that `sync-dependencies` are NOT included in the hash since they only affect local ordering, not Jira content.

---

## Dependency Types Explained

### Why Two Types?

The split allows flexibility:

1. **Same for both** (most common): Task must be created first AND needs Jira link
2. **Sync-only**: Task must be created first, but no Jira link needed
3. **Jira-only**: No creation order requirement, but needs Jira link

### Comparison

| Aspect | sync-dependencies | jira-dependencies |
|--------|-------------------|-------------------|
| **Purpose** | Ticket creation order | Jira "blocks" links |
| **When used** | During `sync` creation phase | During `sync` linking phase |
| **Creates Jira links** | No | Yes |
| **Affects content-hash** | No | Yes |
| **Circular detection** | Yes (error) | No (Jira allows circular) |

### Examples

```yaml
# Most common: Same value for both
sync-dependencies: [KB-1]
jira-dependencies: [KB-1]

# Sync-only: Order matters, but no Jira link needed
sync-dependencies: [KB-1]
jira-dependencies: []

# Jira-only: Link needed, but creation order doesn't matter
sync-dependencies: []
jira-dependencies: [KB-1, ERR-1]

# Different values: Must create after KB-1, but link to both KB-1 and ERR-1
sync-dependencies: [KB-1]
jira-dependencies: [KB-1, ERR-1]
```

---

## CLI Reference

### `jira-sync create`

Create a new markdown task file.

```
Usage:
  jira-sync create [flags]

Flags:
  -t, --title string        Task ID and title (required)
  -p, --parent string       Parent epic/story key (required)
  -d, --description string  Task description (required)
  -s, --sync-deps string    Comma-separated task IDs for creation ordering
  -j, --jira-deps string    Comma-separated task IDs for Jira links
      --deps string         Shorthand: sets both sync-deps and jira-deps
  -o, --output string       Output directory (default "./tasks")
  -h, --help                Help for create
```

### `jira-sync sync`

Synchronize task files with Jira.

```
Usage:
  jira-sync sync [tasks-dir] [flags]

Flags:
  -p, --project string   Default Jira project key (can also be set per-task in frontmatter)
      --dry-run          Show what would happen without making changes
  -y, --yes              Skip confirmation prompts
      --create-only      Only create tickets, don't link dependencies
      --link-only        Only link dependencies, don't create tickets
      --status-only      Only update status from Jira
  -h, --help             Help for sync
```

---

## Sync Workflow

### What Happens During `sync`

```
1. Parse all task files in directory
2. Validate frontmatter and dependencies
3. Topological sort pending tasks by sync-dependencies
   - Detect circular dependencies → error
4. Show summary and prompt for confirmation
5. Create tickets IN TOPOLOGICAL ORDER
   - Update files with jira-number, jira-url
   - Set sync-status to 'created'
6. Link jira-dependencies
   - Create "blocks" links in Jira
   - Set sync-status to 'linked'
7. Show final summary
```

### Output Example

```
Scanning ./tasks/...
Found 47 task files:
  - 4 pending (will create in topological order)
  - 10 created (will link jira-dependencies)
  - 33 linked (up to date)

Pending tickets to create (topological order):
  1. KB-1: Initialize Project (no sync-deps)
  2. KB-2: Create Types (after: KB-1)
  3. KB-3: Create Interfaces (after: KB-2)
  4. CTRL-1: Controller (after: KB-3)

Jira links to create:
  - KB-2 blocked by KB-1
  - KB-3 blocked by KB-2
  - CTRL-1 blocked by KB-3

Sync 47 tasks with Jira? [y/N] y

Creating tickets (in topological order)...
✓ KB-1 → GUARD-101
✓ KB-2 → GUARD-102
✓ KB-3 → GUARD-103
✓ CTRL-1 → GUARD-104

Linking jira-dependencies...
✓ GUARD-102 blocked by GUARD-101
✓ GUARD-103 blocked by GUARD-102
✓ GUARD-104 blocked by GUARD-103

Summary:
  ✓ 4 tickets created
  ✓ 3 jira-dependency links added
  ✓ 47 tasks synced
```

---

## Task ID Prefixes

Recommended prefixes for organizing tasks by area:

| Prefix | Area | Example |
|--------|------|---------|
| KB- | Kubebuilder initialization | KB-1, KB-2, KB-3 |
| CRD- | Custom Resource Definitions | CRD-1 |
| CTRL- | Controller implementation | CTRL-1, CTRL-2 |
| RBAC- | RBAC configuration | RBAC-1, RBAC-2 |
| ERR- | Error detection | ERR-1 through ERR-10 |
| MET- | Metrics & observability | MET-1 through MET-12 |
| HELM- | Helm chart | HELM-1 through HELM-14 |

---

## For AI Assistants (Claude)

When working with task files:

1. **Creating tasks**: Use `jira-sync create` with `--deps` for simple cases
2. **Reading tasks**: Parse YAML frontmatter to understand metadata
3. **Finding dependencies**: Check both `sync-dependencies` and `jira-dependencies`
4. **Checking sync status**: Use `sync-status` field
5. **Linking to Jira**: Use `jira-url` to reference the ticket
6. **Task identification**: Extract task ID from `title` (e.g., "KB-1" from "KB-1: Title")

### Creating Tasks (Recommended Pattern)

```bash
# Use --deps for most tasks (sets both dependency types)
jira-sync create \
  --title "TASK-ID: Task Title" \
  --parent "PARENT-KEY" \
  --deps "DEP-1,DEP-2" \
  --description "Description here"
```

---

## Development

### Running Tests

```bash
go test ./...
```

### Building

```bash
go build -o jira-sync .
```

## License

MIT
