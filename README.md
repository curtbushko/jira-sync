# jira-sync

A Go CLI tool for managing Jira tickets from local markdown files.

## Features

- **Batch ticket creation**: Create multiple Jira tickets from markdown files
- **Dependency linking**: Automatically link dependencies between tickets
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

| Variable | Required | Description |
|----------|----------|-------------|
| `JIRA_TOKEN` | **Yes** | Jira API token |
| `JIRA_URL` | Yes* | Jira instance URL (e.g., `https://company.atlassian.net`) |
| `JIRA_USER` | Yes* | Jira username/email |
| `JIRA_DEFAULTS_PROJECT` | No | Default project key |
| `JIRA_DEFAULTS_ISSUE_TYPE` | No | Default issue type (default: `Task`) |
| `JIRA_DEFAULTS_END_DATE_OFFSET` | No | Days to add for end date (default: `7`) |
| `JIRA_LINK_TYPES_DEPENDENCY` | No | Link type name (default: `Blocks`) |

*Can also be set in config file

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
# Simple task
jira-sync create \
  --title "KB-1: Initialize Project" \
  --parent GUARD-100 \
  --description "Initialize the kubebuilder project"

# Task with dependencies
jira-sync create \
  --title "CTRL-1: Controller Scaffold" \
  --parent GUARD-100 \
  --description "Create the basic controller structure." \
  --dependencies "KB-3,ERR-1"

# Multi-line description with heredoc
jira-sync create \
  --title "ERR-2: Replica Detection" \
  --parent GUARD-100 \
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

## Task File Format

Each task is a markdown file with YAML frontmatter:

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
dependencies: [KB-2, ERR-1]
content-hash: "a1b2c3d4e5f6..."
---

Task description goes here, including acceptance criteria.

## Acceptance Criteria

- First criterion
- Second criterion
```

### Frontmatter Fields

| Field | Type | Description | Set By |
|-------|------|-------------|--------|
| `title` | string | Task ID and title (e.g., "KB-1: Title") | Manual/create |
| `jira-number` | string | Jira issue key (e.g., "GUARD-101") | jira-sync sync |
| `created-date` | date | Date file was created | jira-sync create |
| `start-date` | date | Date ticket created in Jira | jira-sync sync |
| `end-date` | date | start-date + 7 days | jira-sync sync |
| `jira-url` | string | Full URL to Jira issue | jira-sync sync |
| `sync-status` | string | Sync state: pending, created, linked | jira-sync sync |
| `parent` | string | Parent epic/story key | Manual/create |
| `dependencies` | array | List of task IDs this depends on | Manual/create |
| `content-hash` | string | SHA256 hash for change detection | jira-sync sync |

### Sync Status Values

- `pending` - Task file exists, not yet created in Jira
- `created` - Ticket created in Jira, dependencies not linked
- `linked` - Ticket created and all dependencies linked

### Task ID Prefixes

| Prefix | Area | Example |
|--------|------|---------|
| KB- | Kubebuilder initialization | KB-1, KB-2 |
| CRD- | Custom Resource Definitions | CRD-1 |
| CTRL- | Controller implementation | CTRL-1 |
| RBAC- | RBAC configuration | RBAC-1 |
| ERR- | Error detection | ERR-1 |
| MET- | Metrics & observability | MET-1 |
| HELM- | Helm chart | HELM-1 |

## Workflow

### Phase 1: Create Task Files

```bash
mkdir -p jira/tasks

# Create individual task files
jira-sync create \
  --title "KB-1: Initialize Project" \
  --parent GUARD-100 \
  --description "Initialize the kubebuilder project." \
  --output ./jira/tasks/
```

### Phase 2: Review Task Files

```bash
ls -la jira/tasks/
cat jira/tasks/20260116-103001.md
```

### Phase 3: Sync with Jira

```bash
# Dry run first
jira-sync sync ./jira/tasks/ --project GUARD --dry-run

# Run actual sync
jira-sync sync ./jira/tasks/ --project GUARD
```

### Phase 4: Ongoing Sync

```bash
# Add new tasks and sync them
jira-sync create -t "NEW-1: New Feature" -p GUARD-100 -d "New feature" -o ./jira/tasks/
jira-sync sync ./jira/tasks/ --project GUARD
```

## For AI Assistants (Claude)

When working with task files:

1. **Creating tasks**: Use the `jira-sync create` command with flags
2. **Reading tasks**: Parse YAML frontmatter to understand metadata
3. **Finding dependencies**: Look at the `dependencies` array
4. **Checking sync status**: Use `sync-status` field
5. **Linking to Jira**: Use `jira-url` to reference the ticket
6. **Task identification**: Extract task ID from `title` (e.g., "KB-1" from "KB-1: Title")

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
