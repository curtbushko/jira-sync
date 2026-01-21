---
title: Jira Ticket Management Plan
date: 2026-01-16 10:30
tags:
  - jira
  - planning
  - tooling
---

# Jira Ticket Management Plan

## Decision: Custom Go CLI vs jira-cli

### Recommendation: Custom Go CLI

After analyzing the requirements, a **custom Go CLI** is the better approach for this use case.

### Rationale

**Why not jira-cli (bash)?**

1. **Complex file operations**: Reading/writing markdown files with YAML frontmatter is cumbersome in bash
2. **Dependency handling**: The two-phase approach (create all tickets → update dependencies) requires tracking state between operations
3. **Batch operations**: Need transaction-like behavior with rollback on failure
4. **Error handling**: Go provides better error handling than bash scripts
5. **JSON parsing**: jira-cli outputs JSON, but parsing nested structures in bash is error-prone
6. **Field mapping**: Custom fields (start-date, end-date) require API calls that jira-cli may not support directly

**Why Go CLI?**

1. **Native YAML parsing**: `gopkg.in/yaml.v3` handles frontmatter perfectly
2. **Jira API client**: `github.com/andygrunwald/go-jira` is well-maintained
3. **Better UX**: Interactive confirmations, progress bars, colored output
4. **Testable**: Can write unit tests for parsing and creation logic
5. **Reusable**: Can extend for future ticket management needs
6. **Atomic operations**: Can validate all data before creating anything

---

## Architecture Overview

```
jira-sync/
├── cmd/
│   ├── root.go               # Cobra root command + Viper config
│   ├── create.go             # create command (generates .md task files)
│   ├── sync.go               # sync command (syncs with Jira)
│   ├── fullsync.go           # full-sync command (bidirectional sync)
│   └── migrate.go            # migrate command (adds missing frontmatter fields)
├── internal/
│   ├── config/
│   │   └── config.go         # Viper configuration loading
│   ├── jira/
│   │   ├── client.go         # Jira API client wrapper
│   │   ├── issue.go          # Issue creation/update
│   │   └── link.go           # Dependency linking
│   ├── markdown/
│   │   ├── parser.go         # Parse markdown with frontmatter
│   │   ├── writer.go         # Create/update markdown files
│   │   └── types.go          # TaskFile struct
│   └── sync/
│       ├── orchestrator.go   # Full sync orchestration
│       ├── creator.go        # Batch ticket creation
│       ├── linker.go         # Dependency linking
│       └── status.go         # Status updates from Jira
├── main.go                   # Entry point
├── go.mod
└── go.sum
```

---

## Task File Format

Each task will be stored as a markdown file with the following format:

```markdown
---
title: "KB-2: Kubebuilder - Create Shared Types"
jira-number: ""
jira-project: GUARD
jira-state: Todo
created-date: 2026-01-16
start-date: 2026-01-16
end-date: 2026-01-23
jira-url: ""
sync-status: pending
jira-parent: GUARDIAN
sync-dependencies:
  - "[KB-1: Initialize Project](20260116-103001.md)"
jira-dependencies:
  - "[KB-1: Initialize Project](20260116-103001.md)"
content-hash: ""
last-synced: ""
---

Create shared type definitions that will be used across multiple controllers.

## Acceptance Criteria

- DeploymentError type defined with required fields
- ErrorType enum with all error categories
- Unit tests for type validation
- Documentation comments on all exported types
```

**Note:** Dependencies use wiki-style markdown links `[Title](filename.md)` for easy navigation in editors like Obsidian and VS Code.

**Example with no dependencies (root task):**
```markdown
---
title: "KB-1: Kubebuilder - Initialize Project and Repository"
jira-number: ""
jira-project: GUARD
jira-state: Todo
created-date: 2026-01-16
start-date: 2026-01-16
end-date: 2026-01-23
jira-url: ""
sync-status: pending
jira-parent: GUARDIAN
sync-dependencies: []
jira-dependencies: []
content-hash: ""
last-synced: ""
---

Initialize the Kubebuilder project using the CLI tool, create the basic Go module structure, and push to the remote repository. This is the first task that must be completed to unblock all other team members.

## Acceptance Criteria

- Kubebuilder project initialized with `kubebuilder init --domain example.com --repo github.com/org/deployment-error-operator`
- Go module created with go.mod and go.sum
- Basic Makefile exists with default kubebuilder targets
- .gitignore configured for Go projects
- Repository pushed to remote with initial commit
- README.md with basic project description
- All team members can clone and run `make build` successfully
```

### Change Detection

The `content-hash` field stores a SHA256 hash of the **entire file** (frontmatter + description). This allows `jira-sync` to detect when any part of a task file has been modified and needs to be resynced to Jira.

**What triggers a resync:**
- Description changes
- Title changes
- Jira-dependencies changes
- Parent changes
- Any frontmatter field modification

**Note:** Changes to `sync-dependencies` do NOT trigger a resync since they only affect local creation ordering, not Jira ticket content.

**How it works:**

1. When `jira-sync create` creates a new file, `content-hash` is set to empty string
2. When `jira-sync sync` syncs a task to Jira, it:
   - Computes SHA256 of the entire file (excluding the `content-hash` field itself)
   - Stores the hash in `content-hash`
   - Updates the Jira ticket
3. On subsequent syncs, it compares:
   - Current hash of file vs stored `content-hash`
   - If different → file changed → needs resync
   - If same → no changes → skip update

**Sync status + content-hash logic:**

| sync-status | content-hash | Action |
|-------------|--------------|--------|
| `pending` | empty | Create new ticket in Jira |
| `created` | empty | Link dependencies, compute hash |
| `linked` | matches | No action needed |
| `linked` | differs | Update Jira ticket (title, description, etc.), recompute hash |

### Frontmatter Fields

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | Task ID and title (e.g., "KB-1: Kubebuilder - Initialize Project") |
| `jira-number` | string | Jira issue key (populated after creation, e.g., "GUARD-123") |
| `jira-project` | string | Jira project key for this task (e.g., "GUARD") |
| `jira-state` | string | Jira ticket state (default: "Todo", e.g., "Todo", "In Progress", "Done") |
| `created-date` | date | Date the local file was created |
| `start-date` | date | Date ticket was created in Jira (auto-set on creation) |
| `end-date` | date | Start date + 7 days (auto-calculated) |
| `jira-url` | string | Full URL to Jira issue (populated after creation) |
| `sync-status` | string | Sync state: pending, created, linked (tracks sync with Jira, not ticket status) |
| `jira-parent` | string | Parent epic/story key (e.g., "GUARDIAN" or "GUARD-100") |
| `sync-dependencies` | array | Wiki links to tasks that must be created BEFORE this task (controls creation order, e.g., ["[KB-1: Title](20260116-103001.md)"]) |
| `jira-dependencies` | array | Wiki links to tasks this task is blocked by (creates "blocks" links in Jira, e.g., ["[KB-1: Title](file.md)"]) |
| `content-hash` | string | SHA256 hash of entire file (used to detect changes needing resync) |
| `last-synced` | datetime | ISO 8601 timestamp of last successful full-sync (used for conflict detection) |

### Frontmatter Migration (Legacy Task Support)

Older task files may be missing some frontmatter fields that were added in later versions. When parsing task files, `jira-sync` automatically detects and adds missing fields with sensible defaults.

**Missing Field Detection:**

When a task file is parsed, the following fields are checked and added if missing:

| Field | Default Value | Notes |
|-------|---------------|-------|
| `jira-number` | `""` | Empty string (not yet created) |
| `jira-project` | `""` | **Warning**: Must be set manually before sync |
| `jira-state` | `"Todo"` | Default state for new tickets |
| `jira-url` | `""` | Empty string (populated on creation) |
| `sync-status` | `"pending"` | Assumes not yet synced |
| `sync-dependencies` | `[]` | Empty array |
| `jira-dependencies` | `[]` | Empty array |
| `content-hash` | `""` | Empty string (will be computed on sync) |
| `last-synced` | `""` | Empty string (never synced) |

**Migration Behavior:**

1. **On Parse**: When `ParseTaskFile()` reads a task, it calls `MigrateFrontmatter()` to add any missing fields
2. **On Write**: The complete frontmatter (with all fields) is written back to the file
3. **Validation**: If `jira-project` is missing, a warning is displayed during sync (task cannot be created without a project)
4. **Backwards Compatible**: Existing fields are never modified during migration

**Example Migration:**

Old task file (missing fields):
```yaml
---
title: "KB-1: Initialize Project"
jira-parent: GUARD-100
---

Task description here.
```

After migration:
```yaml
---
title: "KB-1: Initialize Project"
jira-number: ""
jira-project: ""
jira-state: Todo
created-date: ""
start-date: ""
end-date: ""
jira-url: ""
sync-status: pending
jira-parent: GUARD-100
sync-dependencies: []
jira-dependencies: []
content-hash: ""
last-synced: ""
---

Task description here.
```

**CLI Flag for Migration:**

```bash
# Migrate all task files without syncing
jira-sync migrate ./tasks/

# Dry run to see what would be migrated
jira-sync migrate ./tasks/ --dry-run

# Migrate with specific project for tasks missing jira-project
jira-sync migrate ./tasks/ --default-project GUARD
```

### Dependency Types Explained

The two dependency fields serve different purposes:

**`sync-dependencies`** - Controls local file creation ordering
- Determines the order in which tickets are created in Jira
- Uses topological sort to ensure dependencies are created first
- Does NOT create any links in Jira
- Uses wiki URL format for easy navigation: `[Task Title](filename.md)`
- Example: If `KB-2` has `sync-dependencies: ["[KB-1: Initialize Project](20260116-103001.md)"]`, KB-1's ticket will be created before KB-2's

**`jira-dependencies`** - Controls Jira ticket linking
- Creates "blocks" links between Jira tickets after creation
- The listed tasks "block" the current task (must complete first)
- Does NOT affect creation order
- Uses wiki URL format for easy navigation: `[Task Title](filename.md)`
- Example: If `KB-2` has `jira-dependencies: ["[KB-1: Initialize Project](20260116-103001.md)"]`, a "GUARD-101 blocks GUARD-102" link is created

### Wiki URL Format for Dependencies

Dependencies use markdown wiki-style links for easy navigation in editors and tools like Obsidian, VS Code, or other markdown viewers.

**Format:** `[Task ID: Title](filename.md)`

**Examples:**
```yaml
sync-dependencies:
  - "[KB-1: Initialize Project](20260116-103001.md)"
  - "[KB-2: Create Types](20260116-103002.md)"
jira-dependencies:
  - "[KB-1: Initialize Project](20260116-103001.md)"
```

**Benefits:**
- Clickable links in markdown editors (Obsidian, VS Code, etc.)
- Easy to see task titles without opening files
- Compatible with wiki-style navigation tools
- Task ID is preserved at the start for parsing

**Parsing Logic:**
- The task ID is extracted from the link text (before the colon)
- The filename is used to locate the actual task file
- If the file doesn't exist, a warning is shown during sync
- Legacy format (plain task IDs like `"KB-1"`) is still supported for backwards compatibility

**Link Resolution:**
```
"[KB-1: Initialize Project](20260116-103001.md)"
  │                          │
  └─ Task ID: KB-1           └─ File: ./tasks/20260116-103001.md
```

**Common patterns:**

1. **Same dependencies** - Most tasks will have identical values:
   ```yaml
   sync-dependencies:
     - "[KB-1: Initialize Project](20260116-103001.md)"
   jira-dependencies:
     - "[KB-1: Initialize Project](20260116-103001.md)"
   ```

2. **Sync-only dependency** - Task needs another created first, but no Jira link needed:
   ```yaml
   sync-dependencies:
     - "[KB-1: Initialize Project](20260116-103001.md)"
   jira-dependencies: []
   ```

3. **Jira-only dependency** - Task can be created in any order, but needs Jira link:
   ```yaml
   sync-dependencies: []
   jira-dependencies:
     - "[KB-1: Initialize Project](20260116-103001.md)"
   ```

4. **Legacy format** - Plain task IDs (still supported):
   ```yaml
   sync-dependencies: ["KB-1"]
   jira-dependencies: ["KB-1"]
   ```

---

## CLI Commands

The CLI has four main commands designed for easy use by both humans and Claude:

### 1. Create Task File

Create a new markdown task file with proper frontmatter. This command is designed to be easily called by Claude when generating tickets.

```bash
jira-sync create [flags]
```

**Flags:**

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--title` | `-t` | Yes | Task ID and title (e.g., "KB-1: Initialize Project") |
| `--jira-parent` | `-p` | Yes | Parent epic/story key (e.g., "GUARD-100") |
| `--project` | | Yes | Jira project key (e.g., "GUARD") |
| `--description` | `-d` | Yes | Task description including acceptance criteria (can be multi-line, becomes Jira description) |
| `--state` | | No | Jira ticket state (default: "Todo") |
| `--sync-deps` | `-s` | No | Comma-separated task IDs for creation ordering (e.g., "KB-1,ERR-1") |
| `--jira-deps` | `-j` | No | Comma-separated task IDs for Jira "blocks" links (e.g., "KB-1,ERR-1") |
| `--deps` | | No | Shorthand: sets BOTH sync-deps and jira-deps to the same value |
| `--output` | `-o` | No | Output directory (default: `./tasks/`) |

**Examples:**

```bash
# Simple task with no dependencies
jira-sync create \
  --title "KB-1: Kubebuilder - Initialize Project and Repository" \
  --jira-parent "GUARD-100" \
  --project "GUARD" \
  --description "Initialize the Kubebuilder project using the CLI tool."

# Task with dependencies (shorthand sets both sync and jira deps)
jira-sync create \
  --title "CTRL-1: Create Basic Controller Scaffold" \
  --jira-parent "GUARD-100" \
  --project "GUARD" \
  --deps "KB-3,ERR-1" \
  --description "Create the basic Deployment controller structure. Controller compiles and can be instantiated in tests."

# Task with separate sync and jira dependencies
jira-sync create \
  --title "ERR-5: Implement Pod Listing" \
  --jira-parent "GUARD-100" \
  --project "GUARD" \
  --sync-deps "KB-3" \
  --jira-deps "KB-3,ERR-1" \
  --description "Implement pod listing by deployment."

# Multi-line description with acceptance criteria using heredoc (for Claude)
jira-sync create \
  --title "ERR-2: Implement Replica Failure Detection" \
  --jira-parent "GUARD-100" \
  --project "GUARD" \
  --deps "ERR-1" \
  --description "$(cat <<'EOF'
Implement detection of replica failures by examining the Deployment status.

Check deployment.Status.UnavailableReplicas and create appropriate errors.

## Acceptance Criteria
- Check if ReplicaFailure detection is enabled
- Read deployment.Status.UnavailableReplicas
- Create DeploymentError when count > 0
- Unit tests with 0, 1, and multiple unavailable replicas
EOF
)"
```

**Output:**

Creates a file like `./tasks/20260116-103001.md`:

```yaml
---
title: "KB-1: Kubebuilder - Initialize Project and Repository"
jira-number: ""
jira-project: GUARD
jira-state: Todo
created-date: 2026-01-16
start-date: ""
end-date: ""
jira-url: ""
sync-status: pending
jira-parent: GUARD-100
sync-dependencies: []
jira-dependencies: []
content-hash: ""
last-synced: ""
---

Initialize the Kubebuilder project using the CLI tool.
```

### 2. Sync with Jira

Synchronize all task files with Jira. This command handles the full lifecycle:
- Creates tickets for `pending` tasks
- Links dependencies for `created` tasks
- Updates local files with Jira data

```bash
jira-sync sync [tasks-dir] [flags]
```

**Flags:**

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--dry-run` | | No | Show what would happen without making changes |
| `--yes` | `-y` | No | Skip confirmation prompts |
| `--create-only` | | No | Only create tickets, don't link dependencies |
| `--link-only` | | No | Only link dependencies, don't create tickets |
| `--status-only` | | No | Only update status from Jira, don't create/link |

Note: The Jira project is read from each task file's `jira-project` frontmatter field.

**Examples:**

```bash
# Full sync (create tickets + link dependencies)
jira-sync sync ./tasks/

# Dry run to see what would happen
jira-sync sync ./tasks/ --dry-run

# Skip confirmation prompts (for scripting)
jira-sync sync ./tasks/ --yes

# Only create tickets (don't link dependencies yet)
jira-sync sync ./tasks/ --create-only

# Only link dependencies (tickets already created)
jira-sync sync ./tasks/ --link-only

# Only update local files from Jira status
jira-sync sync ./tasks/ --status-only
```

**Sync Workflow:**

```
1. Parse all task files in directory
2. Validate frontmatter and dependencies
3. Topological sort pending tasks by sync-dependencies
   - Detect circular dependencies → error
   - Determine creation order
4. Show summary:
   - X tasks pending (will create in topological order)
   - Y tasks created (will link jira-dependencies)
   - Z tasks linked (up to date)
5. Prompt for confirmation
6. Create tickets for pending tasks IN TOPOLOGICAL ORDER
   - Process tasks with no sync-dependencies first
   - Then tasks whose sync-dependencies are satisfied
   - Update local files with jira-number, jira-url
   - Set sync-status to 'created'
7. Link jira-dependencies for created tasks
   - Create "blocks" links in Jira based on jira-dependencies
   - Set sync-status to 'linked'
8. Show final summary
```

**Output Example:**

```
Scanning ./tasks/...
Found 47 task files:
  - 4 pending (will create tickets)
  - 10 created (will link jira-dependencies)
  - 33 linked (up to date)

Pending tickets to create (topological order by sync-dependencies):
  1. KB-1: Kubebuilder - Initialize Project and Repository (no sync-deps)
  2. KB-2: Create Shared Type Definitions (after: KB-1)
  3. KB-3: Create Shared Interfaces (after: KB-2)
  4. KB-4: Create Mock Implementations (after: KB-3)

Jira links to create (from jira-dependencies):
  - KB-2 blocked by KB-1
  - KB-3 blocked by KB-2
  - KB-4 blocked by KB-3
  - ... (7 more)

Sync 47 tasks with Jira? [y/N] y

Creating tickets (in topological order)...
✓ KB-1 → GUARD-101
✓ KB-2 → GUARD-102
✓ KB-3 → GUARD-103
✓ KB-4 → GUARD-104

Linking jira-dependencies...
✓ GUARD-102 blocked by GUARD-101
✓ GUARD-103 blocked by GUARD-102
✓ GUARD-104 blocked by GUARD-103

Summary:
  ✓ 4 tickets created (in dependency order)
  ✓ 10 jira-dependency links added
  ✓ 47 tasks synced
```

### 3. Full Sync (Bidirectional)

The `sync` command is **one-way** (local → Jira) and only handles initial ticket creation and dependency linking. It does NOT keep fields synchronized after creation.

The `full-sync` command provides **bidirectional synchronization** between local task files and Jira tickets, ensuring all fields match.

```bash
jira-sync full-sync [tasks-dir] [flags]
```

**Flags:**

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--dry-run` | | No | Show what would change without making changes |
| `--yes` | `-y` | No | Skip confirmation prompts |
| `--direction` | `-d` | No | Sync direction: `local` (local wins), `jira` (Jira wins), `ask` (prompt for conflicts). Default: `ask` |
| `--fields` | `-f` | No | Comma-separated fields to sync (default: all). Options: `title`, `description`, `state`, `jira-parent` |

**Fields Synchronized:**

| Local Field | Jira Field | Notes |
|-------------|------------|-------|
| `title` | `summary` | Task title |
| Description (body) | `description` | Markdown body content |
| `jira-state` | `status` | Ticket status (Todo, In Progress, Done, etc.) |
| `jira-parent` | `parent` | Parent epic/story |
| `start-date` | Custom field | Start date |
| `end-date` | Custom field | End date |

**Examples:**

```bash
# Full bidirectional sync (prompts on conflicts)
jira-sync full-sync ./tasks/

# Dry run to see what would change
jira-sync full-sync ./tasks/ --dry-run

# Local files always win on conflicts
jira-sync full-sync ./tasks/ --direction local

# Jira always wins on conflicts
jira-sync full-sync ./tasks/ --direction jira

# Only sync specific fields
jira-sync full-sync ./tasks/ --fields "title,description,state"

# Skip confirmation prompts
jira-sync full-sync ./tasks/ --yes
```

**Full-Sync Workflow:**

```
1. Parse all task files in directory
2. Filter to tasks with jira-number (already created in Jira)
3. For each task:
   a. Fetch current Jira ticket data
   b. Compare local fields vs Jira fields
   c. Detect changes:
      - Local only changed → update Jira
      - Jira only changed → update local file
      - Both changed → CONFLICT
4. Show summary of changes:
   - X tasks: local → Jira (local changed)
   - Y tasks: Jira → local (Jira changed)
   - Z tasks: CONFLICT (both changed)
   - W tasks: in sync (no changes)
5. Handle conflicts based on --direction flag:
   - ask: prompt user for each conflict
   - local: local file wins
   - jira: Jira ticket wins
6. Prompt for confirmation
7. Apply changes
8. Update content-hash in local files
9. Show final summary
```

**Conflict Detection:**

Changes are detected by comparing:
1. **Local changes**: Current file hash vs stored `content-hash`
2. **Jira changes**: Current Jira `updated` timestamp vs stored `last-synced` timestamp

A conflict occurs when BOTH local file AND Jira ticket have changed since the last sync.

**Output Example:**

```
Scanning ./tasks/...
Found 47 task files with Jira tickets

Comparing fields...

Changes detected:
  Local → Jira (local file changed):
    - KB-2: description updated
    - ERR-3: title updated

  Jira → Local (Jira ticket changed):
    - CTRL-1: status changed (Todo → In Progress)
    - MET-5: description updated by teammate

  CONFLICTS (both changed):
    - ERR-7: description differs
      Local:  "Implement container terminated state..."
      Jira:   "Updated by PM: Implement container..."
      [L]ocal wins / [J]ira wins / [S]kip?

  In sync: 42 tasks

Apply 4 changes? [y/N] y

Updating Jira...
✓ KB-2: description updated
✓ ERR-3: title updated

Updating local files...
✓ CTRL-1: status → In Progress
✓ MET-5: description updated

Summary:
  ✓ 2 Jira tickets updated
  ✓ 2 local files updated
  ✓ 1 conflict resolved (local wins)
  ✓ 42 tasks already in sync
```

**New Frontmatter Field:**

The `full-sync` command requires an additional frontmatter field to track when the last sync occurred:

| Field | Type | Description |
|-------|------|-------------|
| `last-synced` | datetime | ISO 8601 timestamp of last successful full-sync |

Example:
```yaml
last-synced: "2026-01-21T10:30:00Z"
```

### 4. Migrate (Legacy Task Support)

The `migrate` command adds missing frontmatter fields to older task files. This ensures backwards compatibility when new fields are added to the schema.

```bash
jira-sync migrate [tasks-dir] [flags]
```

**Flags:**

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--dry-run` | | No | Show what would be migrated without making changes |
| `--default-project` | | No | Default jira-project for tasks missing this field |

**Examples:**

```bash
# Migrate all task files in the tasks directory
jira-sync migrate ./tasks/

# Dry run to see what would be migrated
jira-sync migrate ./tasks/ --dry-run

# Set default project for tasks missing jira-project
jira-sync migrate ./tasks/ --default-project GUARD
```

**Output Example:**

```
Scanning ./tasks/ for tasks needing migration...
✓ Migrated: KB-1
✓ Migrated: KB-2
⚠ ERR-1: missing jira-project (use --default-project to set)
✓ Migrated: ERR-2

Migration complete: 3 tasks migrated
1 warnings (missing jira-project)
```

**When to Use:**

- After upgrading jira-sync to a version with new frontmatter fields
- When importing task files from another source
- When manually created task files are missing fields

---

## Workflow

### Phase 1: Create Task Files

Use the `create` command to generate task files. This can be done manually or by Claude.

```bash
# Create tasks directory
mkdir -p jira/tasks

# Create individual task files using the CLI
jira-sync create \
  --title "KB-1: Kubebuilder - Initialize Project and Repository" \
  --jira-parent "GUARD-100" \
  --project "GUARD" \
  --description "Initialize the Kubebuilder project using the CLI tool." \
  --output ./jira/tasks/

jira-sync create \
  --title "KB-2: Kubebuilder - Create Shared Type Definitions" \
  --jira-parent "GUARD-100" \
  --project "GUARD" \
  --description "Create the shared error types and configuration types." \
  --dependencies "KB-1" \
  --output ./jira/tasks/

# Claude can generate many tasks in sequence
jira-sync create -t "KB-3: Create Shared Interfaces" -p GUARD-100 --project GUARD -d "Create interface definitions" --dependencies "KB-2" -o ./jira/tasks/
jira-sync create -t "KB-4: Create Mock Implementations" -p GUARD-100 --project GUARD -d "Create mock implementations" --dependencies "KB-3" -o ./jira/tasks/

# Verify generated files
ls jira/tasks/
# 20260116-103001.md  # KB-1
# 20260116-103002.md  # KB-2
# 20260116-103003.md  # KB-3
# 20260116-103004.md  # KB-4
```

### Phase 2: Review Task Files

Review the generated markdown files to ensure they're correct before syncing.

```bash
# List all task files
ls -la jira/tasks/

# View a specific task
cat jira/tasks/20260116-103001.md
```

### Phase 3: Sync with Jira

Use `sync` to create tickets and link dependencies in one command.

```bash
# Set Jira credentials
export JIRA_URL="https://company.atlassian.net"
export JIRA_USER="user@company.com"
export JIRA_TOKEN="your-api-token"

# Dry run first to see what will happen
jira-sync sync ./jira/tasks/ --dry-run

# Output:
# Scanning ./jira/tasks/...
# Found 47 task files:
#   - 47 pending (will create tickets)
#   - 0 created (will link dependencies)
#   - 0 linked (up to date)
#
# Dry run - no changes will be made
# Would create 47 tickets
# Would create 67 dependency links

# Run the actual sync
jira-sync sync ./jira/tasks/

# Output:
# Scanning ./jira/tasks/...
# Found 47 task files:
#   - 47 pending (will create tickets)
#   - 0 created (will link dependencies)
#   - 0 linked (up to date)
#
# Sync 47 tasks with Jira? [y/N] y
#
# Creating tickets...
# ✓ KB-1 → GUARD-101
# ✓ KB-2 → GUARD-102
# ✓ KB-3 → GUARD-103
# ✓ KB-4 → GUARD-104
# ... (43 more)
#
# Linking dependencies...
# ✓ GUARD-102 blocked by GUARD-101
# ✓ GUARD-103 blocked by GUARD-102
# ✓ GUARD-104 blocked by GUARD-103
# ... (64 more)
#
# ✓ Sync complete
```

### Phase 4: Ongoing Sync

Keep local files in sync with Jira status changes.

```bash
# Update local files from Jira (get latest status)
jira-sync sync ./jira/tasks/ --status-only

# Add new tasks and sync them
jira-sync create -t "NEW-1: New Feature" -p GUARD-100 --project GUARD -d "New feature description" -o ./jira/tasks/
jira-sync sync ./jira/tasks/
```

---

## Go Implementation Details

### Dependencies

```go
// go.mod
module github.com/curtbushko/jira-sync

go 1.21

require (
    github.com/andygrunwald/go-jira v1.16.0  // Jira API client
    github.com/spf13/cobra v1.8.0             // CLI framework
    github.com/spf13/viper v1.18.0            // Configuration management
    github.com/fatih/color v1.16.0            // Colored output
    gopkg.in/yaml.v3 v3.0.1                   // YAML parsing
)
```

### Main Entry Point

```go
// main.go
package main

import "github.com/curtbushko/jira-sync/cmd"

func main() {
    cmd.Execute()
}
```

### Cobra Root Command with Viper

```go
// cmd/root.go
package cmd

import (
    "fmt"
    "os"
    "strings"

    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
    Use:   "jira-sync",
    Short: "Sync markdown task files with Jira",
    Long: `jira-sync manages Jira tickets from local markdown files.

It supports batch creation, dependency linking, and two-way sync
between local task files and Jira issues.`,
}

func Execute() {
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}

func init() {
    cobra.OnInitialize(initConfig)

    // Global flags
    rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
        "config file (default is $HOME/.jira-sync.yaml)")

    // Jira connection flags (can be overridden by env vars)
    rootCmd.PersistentFlags().String("jira-url", "", "Jira instance URL")
    rootCmd.PersistentFlags().String("jira-user", "", "Jira username/email")

    // Bind flags to viper
    viper.BindPFlag("jira.url", rootCmd.PersistentFlags().Lookup("jira-url"))
    viper.BindPFlag("jira.user", rootCmd.PersistentFlags().Lookup("jira-user"))
}

func initConfig() {
    if cfgFile != "" {
        // Use config file from the flag
        viper.SetConfigFile(cfgFile)
    } else {
        // Search for config in home directory
        home, err := os.UserHomeDir()
        cobra.CheckErr(err)

        viper.AddConfigPath(home)
        viper.AddConfigPath(".")
        viper.SetConfigType("yaml")
        viper.SetConfigName(".jira-sync")
    }

    // Environment variable support
    viper.SetEnvPrefix("JIRA")
    viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
    viper.AutomaticEnv()

    // Read config file (ignore if not found)
    if err := viper.ReadInConfig(); err == nil {
        fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
    }

    // Validate required settings
    validateConfig()
}

func validateConfig() {
    // JIRA_TOKEN is required and must come from environment
    if viper.GetString("token") == "" {
        fmt.Fprintln(os.Stderr, "Error: JIRA_TOKEN environment variable is required")
        fmt.Fprintln(os.Stderr, "Set it with: export JIRA_TOKEN='your-api-token'")
        os.Exit(1)
    }
}
```

### Configuration Package

```go
// internal/config/config.go
package config

import (
    "fmt"
    "time"

    "github.com/spf13/viper"
)

// Config holds all configuration for jira-sync
type Config struct {
    Jira     JiraConfig
    Defaults DefaultsConfig
    LinkTypes LinkTypesConfig
}

// JiraConfig holds Jira connection settings
type JiraConfig struct {
    URL   string // From JIRA_URL env var or config file
    User  string // From JIRA_USER env var or config file
    Token string // From JIRA_TOKEN env var ONLY (never in config file)
}

// DefaultsConfig holds default values for ticket creation
type DefaultsConfig struct {
    IssueType       string
    JiraState       string // default jira-state for new tasks (default: "Todo")
    StartDateOffset int    // days from creation
    EndDateOffset   int    // days from start
}

// LinkTypesConfig holds Jira link type names
type LinkTypesConfig struct {
    Dependency string // e.g., "Blocks"
}

// Load returns the current configuration from Viper
func Load() (*Config, error) {
    cfg := &Config{
        Jira: JiraConfig{
            URL:   viper.GetString("jira.url"),
            User:  viper.GetString("jira.user"),
            Token: viper.GetString("token"), // JIRA_TOKEN env var
        },
        Defaults: DefaultsConfig{
            IssueType:       viper.GetString("defaults.issue_type"),
            JiraState:       viper.GetString("defaults.jira_state"),
            StartDateOffset: viper.GetInt("defaults.start_date_offset"),
            EndDateOffset:   viper.GetInt("defaults.end_date_offset"),
        },
        LinkTypes: LinkTypesConfig{
            Dependency: viper.GetString("link_types.dependency"),
        },
    }

    // Apply defaults
    if cfg.Defaults.IssueType == "" {
        cfg.Defaults.IssueType = "Task"
    }
    if cfg.Defaults.JiraState == "" {
        cfg.Defaults.JiraState = "Todo"
    }
    if cfg.Defaults.EndDateOffset == 0 {
        cfg.Defaults.EndDateOffset = 7
    }
    if cfg.LinkTypes.Dependency == "" {
        cfg.LinkTypes.Dependency = "Blocks"
    }

    // Validate required fields
    if cfg.Jira.URL == "" {
        return nil, fmt.Errorf("jira.url is required (set JIRA_URL or use config file)")
    }
    if cfg.Jira.User == "" {
        return nil, fmt.Errorf("jira.user is required (set JIRA_USER or use config file)")
    }
    if cfg.Jira.Token == "" {
        return nil, fmt.Errorf("JIRA_TOKEN environment variable is required")
    }

    return cfg, nil
}

// CalculateEndDate returns start date + EndDateOffset days
func (c *Config) CalculateEndDate(startDate time.Time) time.Time {
    return startDate.AddDate(0, 0, c.Defaults.EndDateOffset)
}
```

### Create Command (Generate Task Files)

This command creates markdown task files - designed for Claude to easily generate tickets.

```go
// cmd/create.go
package cmd

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/curtbushko/jira-sync/internal/markdown"
    "github.com/fatih/color"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

var createCmd = &cobra.Command{
    Use:   "create",
    Short: "Create a new task file",
    Long: `Create a new markdown task file with proper frontmatter.

This command generates a task file that can later be synced to Jira.
Designed for easy use by Claude when generating tickets.

Example:
  jira-sync create --title "KB-1: Initialize Project" --jira-parent GUARD-100 --description "Initialize kubebuilder"
  jira-sync create -t "ERR-1: Detector Stub" -p GUARD-100 -d "Create stub" --deps "KB-3"
  jira-sync create -t "ERR-2: Pod Listing" -p GUARD-100 -d "List pods" --sync-deps "KB-3" --jira-deps "KB-3,ERR-1"`,
    RunE: runCreate,
}

func init() {
    rootCmd.AddCommand(createCmd)

    // Required flags
    createCmd.Flags().StringP("title", "t", "", "Task ID and title (e.g., 'KB-1: Initialize Project')")
    createCmd.Flags().StringP("jira-parent", "p", "", "Parent epic/story key (e.g., 'GUARD-100')")
    createCmd.Flags().StringP("description", "d", "", "Task description including acceptance criteria (becomes Jira description)")

    // Dependency flags
    createCmd.Flags().StringP("sync-deps", "s", "", "Comma-separated task IDs for creation ordering (e.g., 'KB-1,ERR-1')")
    createCmd.Flags().StringP("jira-deps", "j", "", "Comma-separated task IDs for Jira 'blocks' links (e.g., 'KB-1,ERR-1')")
    createCmd.Flags().String("deps", "", "Shorthand: sets BOTH sync-deps and jira-deps to the same value")

    // Other optional flags
    createCmd.Flags().StringP("output", "o", "./tasks", "Output directory for task files")

    // Mark required
    createCmd.MarkFlagRequired("title")
    createCmd.MarkFlagRequired("jira-parent")
    createCmd.MarkFlagRequired("description")

    // Bind output to viper for config file support
    viper.BindPFlag("defaults.output_dir", createCmd.Flags().Lookup("output"))
}

// parseDeps parses a comma-separated dependency string into a slice
func parseDeps(depsStr string) []string {
    var deps []string
    if depsStr != "" {
        for _, d := range strings.Split(depsStr, ",") {
            d = strings.TrimSpace(d)
            if d != "" {
                deps = append(deps, d)
            }
        }
    }
    return deps
}

func runCreate(cmd *cobra.Command, args []string) error {
    // Get flag values
    title, _ := cmd.Flags().GetString("title")
    jiraParent, _ := cmd.Flags().GetString("jira-parent")
    description, _ := cmd.Flags().GetString("description")
    syncDepsStr, _ := cmd.Flags().GetString("sync-deps")
    jiraDepsStr, _ := cmd.Flags().GetString("jira-deps")
    depsStr, _ := cmd.Flags().GetString("deps")
    outputDir, _ := cmd.Flags().GetString("output")

    // Parse dependencies
    // If --deps is set, use it for both; otherwise use individual flags
    var syncDeps, jiraDeps []string
    if depsStr != "" {
        syncDeps = parseDeps(depsStr)
        jiraDeps = parseDeps(depsStr)
    } else {
        syncDeps = parseDeps(syncDepsStr)
        jiraDeps = parseDeps(jiraDepsStr)
    }

    // Ensure output directory exists
    if err := os.MkdirAll(outputDir, 0755); err != nil {
        return fmt.Errorf("create output directory: %w", err)
    }

    // Generate zettelkasten filename
    now := time.Now()
    filename := now.Format("20060102-150405") + ".md"
    filepath := filepath.Join(outputDir, filename)

    // Check if file already exists (unlikely but safe)
    if _, err := os.Stat(filepath); err == nil {
        // Add milliseconds to make unique
        time.Sleep(time.Millisecond)
        now = time.Now()
        filename = now.Format("20060102-150405") + ".md"
        filepath = filepath.Join(outputDir, filename)
    }

    // Create task file struct
    task := &markdown.TaskFile{
        Path: filepath,
        Frontmatter: markdown.Frontmatter{
            Title:            title,
            JiraNumber:       "",
            CreatedDate:      now.Format("2006-01-02"),
            StartDate:        "",
            EndDate:          "",
            JiraURL:          "",
            SyncStatus:       markdown.SyncStatusPending,
            JiraParent:       jiraParent,
            SyncDependencies: syncDeps,
            JiraDependencies: jiraDeps,
            ContentHash:      "",
        },
        Description: description,
    }

    // Write the file
    if err := markdown.WriteTaskFile(task); err != nil {
        return fmt.Errorf("write task file: %w", err)
    }

    color.Green("✓ Created: %s", filepath)
    fmt.Printf("  Title: %s\n", title)
    fmt.Printf("  Jira-Parent: %s\n", jiraParent)
    if len(syncDeps) > 0 {
        fmt.Printf("  Sync-Dependencies: %s\n", strings.Join(syncDeps, ", "))
    }
    if len(jiraDeps) > 0 {
        fmt.Printf("  Jira-Dependencies: %s\n", strings.Join(jiraDeps, ", "))
    }

    return nil
}
```

### Sync Command (Sync with Jira)

```go
// cmd/sync.go
package cmd

import (
    "fmt"

    "github.com/curtbushko/jira-sync/internal/config"
    "github.com/curtbushko/jira-sync/internal/jira"
    "github.com/curtbushko/jira-sync/internal/markdown"
    "github.com/curtbushko/jira-sync/internal/sync"
    "github.com/fatih/color"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

var syncCmd = &cobra.Command{
    Use:   "sync [tasks-dir]",
    Short: "Sync task files with Jira",
    Long: `Synchronize all task files with Jira.

This command handles the full lifecycle:
- Creates tickets for 'pending' tasks
- Links dependencies for 'created' tasks
- Updates local files with Jira data

The Jira project is read from each task file's jira-project frontmatter field.

Example:
  jira-sync sync ./tasks/`,
    Args: cobra.MaximumNArgs(1),
    RunE: runSync,
}

func init() {
    rootCmd.AddCommand(syncCmd)

    syncCmd.Flags().Bool("dry-run", false, "Show what would happen without making changes")
    syncCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
    syncCmd.Flags().Bool("create-only", false, "Only create tickets, don't link dependencies")
    syncCmd.Flags().Bool("link-only", false, "Only link dependencies, don't create tickets")
    syncCmd.Flags().Bool("status-only", false, "Only update status from Jira")
}

func runSync(cmd *cobra.Command, args []string) error {
    // Get tasks directory
    tasksDir := "./tasks"
    if len(args) > 0 {
        tasksDir = args[0]
    }

    // Load configuration
    cfg, err := config.Load()
    if err != nil {
        return fmt.Errorf("load config: %w", err)
    }

    // Get flags
    dryRun, _ := cmd.Flags().GetBool("dry-run")
    skipConfirm, _ := cmd.Flags().GetBool("yes")
    createOnly, _ := cmd.Flags().GetBool("create-only")
    linkOnly, _ := cmd.Flags().GetBool("link-only")
    statusOnly, _ := cmd.Flags().GetBool("status-only")

    // Parse task files
    color.Cyan("Scanning %s...\n", tasksDir)
    tasks, err := markdown.ParseDirectory(tasksDir)
    if err != nil {
        return fmt.Errorf("parse tasks: %w", err)
    }

    // Categorize by sync status and detect changes
    var pending, created, linked, needsUpdate []*markdown.TaskFile
    for _, t := range tasks {
        switch t.Frontmatter.SyncStatus {
        case markdown.SyncStatusPending:
            pending = append(pending, t)
        case markdown.SyncStatusCreated:
            created = append(created, t)
        case markdown.SyncStatusLinked:
            if t.NeedsResync() {
                needsUpdate = append(needsUpdate, t)
            } else {
                linked = append(linked, t)
            }
        }
    }

    // Topological sort pending tasks by sync-dependencies
    sortedPending, err := sync.TopologicalSort(pending, tasks)
    if err != nil {
        return fmt.Errorf("dependency error: %w", err)
    }

    // Show summary
    fmt.Printf("Found %d task files:\n", len(tasks))
    fmt.Printf("  - %d pending (will create in topological order)\n", len(sortedPending))
    fmt.Printf("  - %d created (will link jira-dependencies)\n", len(created))
    fmt.Printf("  - %d modified (will update Jira description)\n", len(needsUpdate))
    fmt.Printf("  - %d linked (up to date)\n", len(linked))
    fmt.Println()

    if statusOnly {
        return runStatusSync(cmd.Context(), cfg, tasks, dryRun)
    }

    if dryRun {
        color.Yellow("Dry run - no changes will be made")
        showPendingTickets(sortedPending) // Shows in topological order
        showJiraDependenciesToLink(created, tasks)
        return nil
    }

    // Confirm
    if !skipConfirm {
        fmt.Printf("Sync %d tasks with Jira? [y/N] ", len(tasks))
        var response string
        fmt.Scanln(&response)
        if response != "y" && response != "Y" {
            color.Yellow("Cancelled")
            return nil
        }
    }

    // Create Jira client
    client, err := jira.NewClient(cfg.Jira.URL, cfg.Jira.User, cfg.Jira.Token)
    if err != nil {
        return fmt.Errorf("create jira client: %w", err)
    }

    // Create orchestrator (project is read from each task's jira-project field)
    orch := sync.NewOrchestrator(client, cfg)

    // Phase 1: Create tickets IN TOPOLOGICAL ORDER (respects sync-dependencies)
    if !linkOnly && len(sortedPending) > 0 {
        color.Cyan("\nCreating tickets (in topological order)...\n")
        if err := orch.CreateTickets(cmd.Context(), sortedPending); err != nil {
            return fmt.Errorf("create tickets: %w", err)
        }
        // Move created tickets to the created list for linking
        created = append(created, sortedPending...)
    }

    // Phase 2: Link jira-dependencies
    if !createOnly && len(created) > 0 {
        color.Cyan("\nLinking jira-dependencies...\n")
        if err := orch.LinkJiraDependencies(cmd.Context(), created, tasks); err != nil {
            return fmt.Errorf("link jira-dependencies: %w", err)
        }
    }

    color.Green("\n✓ Sync complete")
    return nil
}
```

### Full-Sync Command (Bidirectional Sync)

```go
// cmd/fullsync.go
package cmd

import (
    "context"
    "fmt"
    "time"

    "github.com/curtbushko/jira-sync/internal/config"
    "github.com/curtbushko/jira-sync/internal/jira"
    "github.com/curtbushko/jira-sync/internal/markdown"
    "github.com/curtbushko/jira-sync/internal/sync"
    "github.com/fatih/color"
    "github.com/spf13/cobra"
)

var fullSyncCmd = &cobra.Command{
    Use:   "full-sync [tasks-dir]",
    Short: "Bidirectional sync between task files and Jira",
    Long: `Bidirectional synchronization between local task files and Jira tickets.

Unlike 'sync' which only creates tickets, full-sync keeps all fields
synchronized between local files and Jira:
- Detects local changes and updates Jira
- Detects Jira changes and updates local files
- Handles conflicts when both have changed

Example:
  jira-sync full-sync ./tasks/
  jira-sync full-sync ./tasks/ --direction local`,
    Args: cobra.MaximumNArgs(1),
    RunE: runFullSync,
}

func init() {
    rootCmd.AddCommand(fullSyncCmd)

    fullSyncCmd.Flags().Bool("dry-run", false, "Show what would change without making changes")
    fullSyncCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
    fullSyncCmd.Flags().StringP("direction", "d", "ask", "Conflict resolution: local, jira, or ask")
    fullSyncCmd.Flags().StringP("fields", "f", "", "Comma-separated fields to sync (default: all)")
}

func runFullSync(cmd *cobra.Command, args []string) error {
    // Get tasks directory
    tasksDir := "./tasks"
    if len(args) > 0 {
        tasksDir = args[0]
    }

    // Load configuration
    cfg, err := config.Load()
    if err != nil {
        return fmt.Errorf("load config: %w", err)
    }

    // Get flags
    dryRun, _ := cmd.Flags().GetBool("dry-run")
    skipConfirm, _ := cmd.Flags().GetBool("yes")
    direction, _ := cmd.Flags().GetString("direction")
    fieldsStr, _ := cmd.Flags().GetString("fields")

    // Validate direction
    if direction != "local" && direction != "jira" && direction != "ask" {
        return fmt.Errorf("--direction must be 'local', 'jira', or 'ask'")
    }

    // Parse task files
    color.Cyan("Scanning %s...\n", tasksDir)
    tasks, err := markdown.ParseDirectory(tasksDir)
    if err != nil {
        return fmt.Errorf("parse tasks: %w", err)
    }

    // Filter to tasks with Jira tickets
    var syncableTasks []*markdown.TaskFile
    for _, t := range tasks {
        if t.Frontmatter.JiraNumber != "" {
            syncableTasks = append(syncableTasks, t)
        }
    }

    fmt.Printf("Found %d task files with Jira tickets\n\n", len(syncableTasks))

    if len(syncableTasks) == 0 {
        color.Yellow("No tasks to sync (all tasks are pending)")
        return nil
    }

    // Create Jira client
    client, err := jira.NewClient(cfg.Jira.URL, cfg.Jira.User, cfg.Jira.Token)
    if err != nil {
        return fmt.Errorf("create jira client: %w", err)
    }

    // Create full-sync service
    syncer := sync.NewFullSyncer(client, cfg, parseFields(fieldsStr))

    // Compare all tasks
    color.Cyan("Comparing fields...\n")
    results, err := syncer.Compare(cmd.Context(), syncableTasks)
    if err != nil {
        return fmt.Errorf("compare tasks: %w", err)
    }

    // Show summary
    showFullSyncSummary(results)

    // Handle conflicts
    if len(results.Conflicts) > 0 && direction == "ask" && !dryRun {
        if err := resolveConflicts(results.Conflicts); err != nil {
            return fmt.Errorf("resolve conflicts: %w", err)
        }
    }

    if dryRun {
        color.Yellow("\nDry run - no changes will be made")
        return nil
    }

    // Count total changes
    totalChanges := len(results.LocalToJira) + len(results.JiraToLocal) + len(results.Conflicts)
    if totalChanges == 0 {
        color.Green("\n✓ All tasks are in sync")
        return nil
    }

    // Confirm
    if !skipConfirm {
        fmt.Printf("\nApply %d changes? [y/N] ", totalChanges)
        var response string
        fmt.Scanln(&response)
        if response != "y" && response != "Y" {
            color.Yellow("Cancelled")
            return nil
        }
    }

    // Apply changes
    if err := syncer.Apply(cmd.Context(), results, direction); err != nil {
        return fmt.Errorf("apply changes: %w", err)
    }

    // Update last-synced timestamp
    now := time.Now().UTC().Format(time.RFC3339)
    for _, t := range syncableTasks {
        t.Frontmatter.LastSynced = now
        if err := markdown.WriteTaskFile(t); err != nil {
            return fmt.Errorf("update last-synced: %w", err)
        }
    }

    color.Green("\n✓ Full sync complete")
    return nil
}

func showFullSyncSummary(results *sync.FullSyncResults) {
    fmt.Println("\nChanges detected:")

    if len(results.LocalToJira) > 0 {
        fmt.Println("  Local → Jira (local file changed):")
        for _, c := range results.LocalToJira {
            fmt.Printf("    - %s: %s\n", c.TaskID, c.Description)
        }
    }

    if len(results.JiraToLocal) > 0 {
        fmt.Println("\n  Jira → Local (Jira ticket changed):")
        for _, c := range results.JiraToLocal {
            fmt.Printf("    - %s: %s\n", c.TaskID, c.Description)
        }
    }

    if len(results.Conflicts) > 0 {
        color.Yellow("\n  CONFLICTS (both changed):")
        for _, c := range results.Conflicts {
            fmt.Printf("    - %s: %s\n", c.TaskID, c.Field)
            fmt.Printf("      Local:  %q\n", truncate(c.LocalValue, 50))
            fmt.Printf("      Jira:   %q\n", truncate(c.JiraValue, 50))
        }
    }

    fmt.Printf("\n  In sync: %d tasks\n", results.InSyncCount)
}
```

### Full-Sync Service

```go
// internal/sync/fullsync.go
package sync

import (
    "context"
    "time"

    "github.com/curtbushko/jira-sync/internal/jira"
    "github.com/curtbushko/jira-sync/internal/markdown"
)

// FullSyncResults holds the comparison results
type FullSyncResults struct {
    LocalToJira []*Change   // Local changed, need to update Jira
    JiraToLocal []*Change   // Jira changed, need to update local
    Conflicts   []*Conflict // Both changed
    InSyncCount int         // Number of tasks already in sync
}

// Change represents a single field change
type Change struct {
    Task        *markdown.TaskFile
    TaskID      string
    Field       string
    Description string
    OldValue    string
    NewValue    string
}

// Conflict represents a field that changed in both local and Jira
type Conflict struct {
    Task       *markdown.TaskFile
    TaskID     string
    Field      string
    LocalValue string
    JiraValue  string
    Resolution string // "local", "jira", or "" (unresolved)
}

// FullSyncer handles bidirectional synchronization
type FullSyncer struct {
    jiraClient *jira.Client
    cfg        *config.Config
    fields     []string // fields to sync, empty = all
}

// NewFullSyncer creates a new full sync service
func NewFullSyncer(client *jira.Client, cfg *config.Config, fields []string) *FullSyncer {
    return &FullSyncer{
        jiraClient: client,
        cfg:        cfg,
        fields:     fields,
    }
}

// Compare compares local tasks with Jira tickets
func (s *FullSyncer) Compare(ctx context.Context, tasks []*markdown.TaskFile) (*FullSyncResults, error) {
    results := &FullSyncResults{}

    for _, task := range tasks {
        // Fetch Jira ticket
        issue, _, err := s.jiraClient.Issue.Get(task.Frontmatter.JiraNumber, nil)
        if err != nil {
            return nil, fmt.Errorf("fetch %s: %w", task.Frontmatter.JiraNumber, err)
        }

        // Check if local changed since last sync
        localChanged := task.NeedsResync()

        // Check if Jira changed since last sync
        jiraChanged := s.jiraChangedSince(issue, task.Frontmatter.LastSynced)

        // Compare fields
        fieldChanges := s.compareFields(task, issue)

        if len(fieldChanges) == 0 {
            results.InSyncCount++
            continue
        }

        for _, fc := range fieldChanges {
            if localChanged && jiraChanged {
                // Conflict: both changed
                results.Conflicts = append(results.Conflicts, &Conflict{
                    Task:       task,
                    TaskID:     task.TaskID(),
                    Field:      fc.Field,
                    LocalValue: fc.LocalValue,
                    JiraValue:  fc.JiraValue,
                })
            } else if localChanged {
                // Local changed, update Jira
                results.LocalToJira = append(results.LocalToJira, &Change{
                    Task:        task,
                    TaskID:      task.TaskID(),
                    Field:       fc.Field,
                    Description: fmt.Sprintf("%s updated", fc.Field),
                    OldValue:    fc.JiraValue,
                    NewValue:    fc.LocalValue,
                })
            } else if jiraChanged {
                // Jira changed, update local
                results.JiraToLocal = append(results.JiraToLocal, &Change{
                    Task:        task,
                    TaskID:      task.TaskID(),
                    Field:       fc.Field,
                    Description: fmt.Sprintf("%s changed", fc.Field),
                    OldValue:    fc.LocalValue,
                    NewValue:    fc.JiraValue,
                })
            }
        }
    }

    return results, nil
}

// compareFields compares individual fields between local and Jira
func (s *FullSyncer) compareFields(task *markdown.TaskFile, issue *jira.Issue) []FieldComparison {
    var diffs []FieldComparison

    // Title/Summary
    if s.shouldSync("title") && task.Frontmatter.Title != issue.Fields.Summary {
        diffs = append(diffs, FieldComparison{
            Field:      "title",
            LocalValue: task.Frontmatter.Title,
            JiraValue:  issue.Fields.Summary,
        })
    }

    // Description
    if s.shouldSync("description") && task.Description != issue.Fields.Description {
        diffs = append(diffs, FieldComparison{
            Field:      "description",
            LocalValue: task.Description,
            JiraValue:  issue.Fields.Description,
        })
    }

    // Status/State
    if s.shouldSync("state") && task.Frontmatter.JiraState != issue.Fields.Status.Name {
        diffs = append(diffs, FieldComparison{
            Field:      "state",
            LocalValue: task.Frontmatter.JiraState,
            JiraValue:  issue.Fields.Status.Name,
        })
    }

    // Parent
    if s.shouldSync("jira-parent") && issue.Fields.Parent != nil {
        if task.Frontmatter.JiraParent != issue.Fields.Parent.Key {
            diffs = append(diffs, FieldComparison{
                Field:      "jira-parent",
                LocalValue: task.Frontmatter.JiraParent,
                JiraValue:  issue.Fields.Parent.Key,
            })
        }
    }

    return diffs
}

// jiraChangedSince checks if Jira ticket was updated after the given timestamp
func (s *FullSyncer) jiraChangedSince(issue *jira.Issue, lastSynced string) bool {
    if lastSynced == "" {
        return true // Never synced, assume changed
    }

    syncTime, err := time.Parse(time.RFC3339, lastSynced)
    if err != nil {
        return true // Invalid timestamp, assume changed
    }

    // Jira's Updated field is in the issue
    jiraUpdated := time.Time(issue.Fields.Updated)
    return jiraUpdated.After(syncTime)
}

// Apply applies the changes based on direction
func (s *FullSyncer) Apply(ctx context.Context, results *FullSyncResults, direction string) error {
    // Apply local → Jira changes
    for _, change := range results.LocalToJira {
        if err := s.updateJira(ctx, change); err != nil {
            return fmt.Errorf("update jira %s: %w", change.TaskID, err)
        }
        color.Green("✓ %s: %s updated in Jira", change.TaskID, change.Field)
    }

    // Apply Jira → local changes
    for _, change := range results.JiraToLocal {
        if err := s.updateLocal(change); err != nil {
            return fmt.Errorf("update local %s: %w", change.TaskID, err)
        }
        color.Green("✓ %s: %s updated locally", change.TaskID, change.Field)
    }

    // Apply resolved conflicts
    for _, conflict := range results.Conflicts {
        if conflict.Resolution == "" {
            continue // Unresolved, skip
        }

        if conflict.Resolution == "local" {
            change := &Change{
                Task:     conflict.Task,
                TaskID:   conflict.TaskID,
                Field:    conflict.Field,
                NewValue: conflict.LocalValue,
            }
            if err := s.updateJira(ctx, change); err != nil {
                return fmt.Errorf("resolve conflict %s: %w", conflict.TaskID, err)
            }
            color.Green("✓ %s: conflict resolved (local wins)", conflict.TaskID)
        } else if conflict.Resolution == "jira" {
            change := &Change{
                Task:     conflict.Task,
                TaskID:   conflict.TaskID,
                Field:    conflict.Field,
                NewValue: conflict.JiraValue,
            }
            if err := s.updateLocal(change); err != nil {
                return fmt.Errorf("resolve conflict %s: %w", conflict.TaskID, err)
            }
            color.Green("✓ %s: conflict resolved (jira wins)", conflict.TaskID)
        }
    }

    return nil
}

// updateJira updates a field in Jira
func (s *FullSyncer) updateJira(ctx context.Context, change *Change) error {
    update := map[string]interface{}{}

    switch change.Field {
    case "title":
        update["summary"] = change.NewValue
    case "description":
        update["description"] = change.NewValue
    case "state":
        // Status changes require transitions, handled separately
        return s.transitionIssue(ctx, change.Task.Frontmatter.JiraNumber, change.NewValue)
    case "jira-parent":
        update["parent"] = map[string]string{"key": change.NewValue}
    }

    if len(update) > 0 {
        _, _, err := s.jiraClient.Issue.Update(&jira.Issue{
            Key:    change.Task.Frontmatter.JiraNumber,
            Fields: &jira.IssueFields{},
        })
        return err
    }

    return nil
}

// updateLocal updates a field in the local task file
func (s *FullSyncer) updateLocal(change *Change) error {
    switch change.Field {
    case "title":
        change.Task.Frontmatter.Title = change.NewValue
    case "description":
        change.Task.Description = change.NewValue
    case "state":
        change.Task.Frontmatter.JiraState = change.NewValue
    case "jira-parent":
        change.Task.Frontmatter.JiraParent = change.NewValue
    }

    // Update content hash
    change.Task.Frontmatter.ContentHash = change.Task.ComputeContentHash()

    return markdown.WriteTaskFile(change.Task)
}
```

### Jira Client with Token Auth

```go
// internal/jira/client.go
package jira

import (
    "fmt"

    jira "github.com/andygrunwald/go-jira"
)

// Client wraps the go-jira client with our configuration
type Client struct {
    *jira.Client
    BaseURL string
}

// NewClient creates a new Jira client using Basic Auth with API token
// The token should come from JIRA_TOKEN environment variable
func NewClient(url, user, token string) (*Client, error) {
    tp := jira.BasicAuthTransport{
        Username: user,
        Password: token, // API token, not password
    }

    client, err := jira.NewClient(tp.Client(), url)
    if err != nil {
        return nil, fmt.Errorf("create jira client: %w", err)
    }

    // Verify connection
    _, _, err = client.User.GetSelf()
    if err != nil {
        return nil, fmt.Errorf("jira authentication failed: %w", err)
    }

    return &Client{
        Client:  client,
        BaseURL: url,
    }, nil
}
```

### Core Types

```go
// internal/markdown/types.go
package markdown

type TaskFile struct {
    Path        string
    Frontmatter Frontmatter
    Description string // Body content (becomes Jira description)
}

type Frontmatter struct {
    Title            string   `yaml:"title"`
    JiraNumber       string   `yaml:"jira-number"`
    JiraProject      string   `yaml:"jira-project"`       // Jira project key (e.g., "GUARD")
    JiraState        string   `yaml:"jira-state"`         // Jira ticket state (default: "Todo")
    CreatedDate      string   `yaml:"created-date"`
    StartDate        string   `yaml:"start-date"`
    EndDate          string   `yaml:"end-date"`
    JiraURL          string   `yaml:"jira-url"`
    SyncStatus       string   `yaml:"sync-status"`        // Tracks sync state, not Jira ticket status
    JiraParent       string   `yaml:"jira-parent"`
    SyncDependencies []string `yaml:"sync-dependencies"`  // Controls creation order (topological sort)
    JiraDependencies []string `yaml:"jira-dependencies"`  // Creates "blocks" links in Jira
    ContentHash      string   `yaml:"content-hash"`       // SHA256 of description for change detection
    LastSynced       string   `yaml:"last-synced"`        // ISO 8601 timestamp of last full-sync
}

// SyncStatus values - tracks sync state between local files and Jira
const (
    SyncStatusPending = "pending" // Not yet created in Jira
    SyncStatusCreated = "created" // Created in Jira, dependencies not linked
    SyncStatusLinked  = "linked"  // Created and dependencies linked
)

// ComputeContentHash returns SHA256 hash of the entire file for change detection
// It hashes title + jira-parent + jira-dependencies + description
// NOTE: sync-dependencies are NOT included since they don't affect Jira content
func (t *TaskFile) ComputeContentHash() string {
    // Build canonical content string (excludes content-hash and sync-dependencies)
    var buf bytes.Buffer
    buf.WriteString(t.Frontmatter.Title)
    buf.WriteString(t.Frontmatter.JiraParent)
    for _, dep := range t.Frontmatter.JiraDependencies {
        buf.WriteString(dep)
    }
    buf.WriteString(t.Description)

    hash := sha256.Sum256(buf.Bytes())
    return hex.EncodeToString(hash[:])
}

// NeedsResync returns true if any part of the file has changed since last sync
func (t *TaskFile) NeedsResync() bool {
    if t.Frontmatter.ContentHash == "" {
        return true // Never synced
    }
    currentHash := t.ComputeContentHash()
    return currentHash != t.Frontmatter.ContentHash
}

// TaskID extracts the task ID from the title (e.g., "KB-1" from "KB-1: Title")
func (t *TaskFile) TaskID() string {
    parts := strings.SplitN(t.Frontmatter.Title, ":", 2)
    if len(parts) > 0 {
        return strings.TrimSpace(parts[0])
    }
    return ""
}
```

### Parser

```go
// internal/markdown/parser.go
package markdown

import (
    "fmt"
    "os"
    "regexp"
    "strings"

    "gopkg.in/yaml.v3"
)

// wikiLinkRegex matches wiki-style links: [Title](filename.md)
var wikiLinkRegex = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+\.md)\)`)

func ParseTaskFile(path string) (*TaskFile, error) {
    content, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    // Split frontmatter and body
    parts := strings.SplitN(string(content), "---", 3)
    if len(parts) < 3 {
        return nil, fmt.Errorf("invalid frontmatter format")
    }

    var fm Frontmatter
    if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
        return nil, fmt.Errorf("parse frontmatter: %w", err)
    }

    task := &TaskFile{
        Path:        path,
        Frontmatter: fm,
        Description: strings.TrimSpace(parts[2]),
    }

    // Migrate missing fields
    task.MigrateFrontmatter()

    return task, nil
}

// MigrateFrontmatter adds missing fields with default values
// This ensures older task files work with newer versions of jira-sync
func (t *TaskFile) MigrateFrontmatter() bool {
    migrated := false

    // Apply defaults for missing fields
    if t.Frontmatter.JiraState == "" {
        t.Frontmatter.JiraState = "Todo"
        migrated = true
    }
    if t.Frontmatter.SyncStatus == "" {
        t.Frontmatter.SyncStatus = SyncStatusPending
        migrated = true
    }
    if t.Frontmatter.SyncDependencies == nil {
        t.Frontmatter.SyncDependencies = []string{}
        migrated = true
    }
    if t.Frontmatter.JiraDependencies == nil {
        t.Frontmatter.JiraDependencies = []string{}
        migrated = true
    }

    return migrated
}

// ParseWikiLink extracts task ID and filename from a wiki-style link
// Supports both formats:
//   - Wiki link: "[KB-1: Initialize Project](20260116-103001.md)" -> ("KB-1", "20260116-103001.md")
//   - Legacy: "KB-1" -> ("KB-1", "")
func ParseWikiLink(link string) (taskID string, filename string) {
    matches := wikiLinkRegex.FindStringSubmatch(link)
    if len(matches) == 3 {
        // Wiki link format: extract task ID from title (before colon)
        title := matches[1]
        filename = matches[2]
        parts := strings.SplitN(title, ":", 2)
        taskID = strings.TrimSpace(parts[0])
        return taskID, filename
    }

    // Legacy format: plain task ID
    return strings.TrimSpace(link), ""
}

// FormatWikiLink creates a wiki-style link from task info
func FormatWikiLink(taskID, title, filename string) string {
    return fmt.Sprintf("[%s: %s](%s)", taskID, title, filename)
}

// GetDependencyTaskIDs extracts task IDs from dependency array (handles both formats)
func (t *TaskFile) GetSyncDependencyIDs() []string {
    var ids []string
    for _, dep := range t.Frontmatter.SyncDependencies {
        taskID, _ := ParseWikiLink(dep)
        if taskID != "" {
            ids = append(ids, taskID)
        }
    }
    return ids
}

// GetJiraDependencyIDs extracts task IDs from jira-dependencies array
func (t *TaskFile) GetJiraDependencyIDs() []string {
    var ids []string
    for _, dep := range t.Frontmatter.JiraDependencies {
        taskID, _ := ParseWikiLink(dep)
        if taskID != "" {
            ids = append(ids, taskID)
        }
    }
    return ids
}
```

### Migrate Command

```go
// cmd/migrate.go
package cmd

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/curtbushko/jira-sync/internal/markdown"
    "github.com/fatih/color"
    "github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
    Use:   "migrate [tasks-dir]",
    Short: "Migrate task files to add missing frontmatter fields",
    Long: `Migrate older task files to include all required frontmatter fields.

This command scans task files and adds any missing fields with sensible defaults.
Existing field values are never modified.

Example:
  jira-sync migrate ./tasks/
  jira-sync migrate ./tasks/ --dry-run
  jira-sync migrate ./tasks/ --default-project GUARD`,
    Args: cobra.MaximumNArgs(1),
    RunE: runMigrate,
}

func init() {
    rootCmd.AddCommand(migrateCmd)

    migrateCmd.Flags().Bool("dry-run", false, "Show what would be migrated without making changes")
    migrateCmd.Flags().String("default-project", "", "Default jira-project for tasks missing this field")
}

func runMigrate(cmd *cobra.Command, args []string) error {
    tasksDir := "./tasks"
    if len(args) > 0 {
        tasksDir = args[0]
    }

    dryRun, _ := cmd.Flags().GetBool("dry-run")
    defaultProject, _ := cmd.Flags().GetString("default-project")

    color.Cyan("Scanning %s for tasks needing migration...\n", tasksDir)

    var migrated, skipped, warnings int

    err := filepath.Walk(tasksDir, func(path string, info os.FileInfo, err error) error {
        if err != nil || info.IsDir() || filepath.Ext(path) != ".md" {
            return err
        }

        task, err := markdown.ParseTaskFile(path)
        if err != nil {
            color.Yellow("⚠ Skipping %s: %v", path, err)
            skipped++
            return nil
        }

        // Check if migration is needed
        needsMigration := task.MigrateFrontmatter()

        // Apply default project if missing
        if task.Frontmatter.JiraProject == "" {
            if defaultProject != "" {
                task.Frontmatter.JiraProject = defaultProject
                needsMigration = true
            } else {
                color.Yellow("⚠ %s: missing jira-project (use --default-project to set)", task.TaskID())
                warnings++
            }
        }

        if !needsMigration {
            return nil
        }

        if dryRun {
            color.Green("Would migrate: %s", task.TaskID())
        } else {
            if err := markdown.WriteTaskFile(task); err != nil {
                return fmt.Errorf("write %s: %w", path, err)
            }
            color.Green("✓ Migrated: %s", task.TaskID())
        }
        migrated++

        return nil
    })

    if err != nil {
        return err
    }

    fmt.Println()
    if dryRun {
        color.Cyan("Dry run complete: %d tasks would be migrated", migrated)
    } else {
        color.Green("Migration complete: %d tasks migrated", migrated)
    }
    if skipped > 0 {
        color.Yellow("%d files skipped (parse errors)", skipped)
    }
    if warnings > 0 {
        color.Yellow("%d warnings (missing jira-project)", warnings)
    }

    return nil
}
```

### Topological Sort (for sync-dependencies)

```go
// internal/sync/toposort.go
package sync

import (
    "fmt"

    "github.com/curtbushko/jira-sync/internal/markdown"
)

// TopologicalSort orders tasks so that sync-dependencies are created first.
// Returns error if circular dependency is detected.
// Supports both wiki link format and legacy plain task ID format.
func TopologicalSort(pending []*markdown.TaskFile, allTasks []*markdown.TaskFile) ([]*markdown.TaskFile, error) {
    // Build task ID to task map (includes all tasks, not just pending)
    taskByID := make(map[string]*markdown.TaskFile)
    for _, t := range allTasks {
        taskByID[t.TaskID()] = t
    }

    // Build adjacency list for pending tasks only
    pendingSet := make(map[string]bool)
    for _, t := range pending {
        pendingSet[t.TaskID()] = true
    }

    // Kahn's algorithm for topological sort
    // Count incoming edges (dependencies) for each pending task
    inDegree := make(map[string]int)
    for _, t := range pending {
        id := t.TaskID()
        inDegree[id] = 0
    }

    // Count sync-dependencies that are also pending
    // Uses GetSyncDependencyIDs() to handle both wiki link and legacy formats
    for _, t := range pending {
        id := t.TaskID()
        for _, depID := range t.GetSyncDependencyIDs() {
            if pendingSet[depID] {
                inDegree[id]++
            }
            // If dependency is not pending, it's already created (or missing)
        }
    }

    // Start with tasks that have no pending dependencies
    var queue []string
    for id, deg := range inDegree {
        if deg == 0 {
            queue = append(queue, id)
        }
    }

    var sorted []*markdown.TaskFile
    for len(queue) > 0 {
        // Pop from queue
        id := queue[0]
        queue = queue[1:]

        task := taskByID[id]
        sorted = append(sorted, task)

        // Reduce in-degree for tasks that depend on this one
        for _, t := range pending {
            for _, depID := range t.GetSyncDependencyIDs() {
                if depID == id {
                    inDegree[t.TaskID()]--
                    if inDegree[t.TaskID()] == 0 {
                        queue = append(queue, t.TaskID())
                    }
                }
            }
        }
    }

    // Check for circular dependency
    if len(sorted) != len(pending) {
        return nil, fmt.Errorf("circular sync-dependency detected")
    }

    return sorted, nil
}
```

### Batch Creation

```go
// internal/sync/batch.go
package sync

import (
    "context"
    "fmt"
    "time"
)

type BatchCreator struct {
    jiraClient *jira.Client
    tasks      []*markdown.TaskFile // Already topologically sorted by sync-dependencies
}

func (b *BatchCreator) Create(ctx context.Context) error {
    // Create issues in order (tasks are pre-sorted by sync-dependencies)
    issueMap := make(map[string]string) // local ID -> Jira key

    for _, task := range b.tasks {
        // Validate that task has required jira-project field
        if task.Frontmatter.JiraProject == "" {
            return fmt.Errorf("task %s is missing required jira-project field", task.Frontmatter.Title)
        }

        issue, err := b.createIssue(ctx, task)
        if err != nil {
            return fmt.Errorf("create %s: %w", task.Frontmatter.Title, err)
        }

        // Extract local ID from title (e.g., "KB-1" from "KB-1: ...")
        localID := task.TaskID()
        issueMap[localID] = issue.Key

        // Update local file
        task.Frontmatter.JiraNumber = issue.Key
        task.Frontmatter.JiraURL = b.jiraClient.BaseURL + "/browse/" + issue.Key
        task.Frontmatter.SyncStatus = markdown.SyncStatusCreated
        task.Frontmatter.StartDate = time.Now().Format("2006-01-02")
        task.Frontmatter.EndDate = time.Now().AddDate(0, 0, 7).Format("2006-01-02")

        if err := markdown.WriteTaskFile(task); err != nil {
            return fmt.Errorf("update file %s: %w", task.Path, err)
        }

        color.Green("✓ %s → %s", localID, issue.Key)
    }

    return nil
}

func (b *BatchCreator) createIssue(ctx context.Context, task *markdown.TaskFile) (*jira.Issue, error) {
    issue := &jira.Issue{
        Fields: &jira.IssueFields{
            Project:     jira.Project{Key: task.Frontmatter.JiraProject},
            Summary:     task.Frontmatter.Title,
            Description: task.Description,
            Type:        jira.IssueType{Name: "Task"},
            Parent:      &jira.Parent{Key: task.Frontmatter.JiraParent},
        },
    }

    created, _, err := b.jiraClient.Issue.Create(issue)
    return created, err
}
```

### Jira Dependency Linking

```go
// internal/sync/dependencies.go
package sync

import (
    "context"
    "fmt"

    "github.com/fatih/color"
)

// JiraDependencyLinker creates "blocks" links in Jira based on jira-dependencies field.
// This is separate from sync-dependencies which only affects creation order.
type JiraDependencyLinker struct {
    jiraClient *jira.Client
    tasks      []*markdown.TaskFile
}

func (d *JiraDependencyLinker) Link(ctx context.Context) error {
    // Build local ID to Jira key map
    idMap := make(map[string]string)
    for _, task := range d.tasks {
        localID := task.TaskID()
        idMap[localID] = task.Frontmatter.JiraNumber
    }

    // Create links based on jira-dependencies (NOT sync-dependencies)
    for _, task := range d.tasks {
        if len(task.Frontmatter.JiraDependencies) == 0 {
            continue
        }

        blockedIssue := task.Frontmatter.JiraNumber
        for _, depID := range task.Frontmatter.JiraDependencies {
            blockerIssue, ok := idMap[depID]
            if !ok {
                return fmt.Errorf("jira-dependency %s not found for %s", depID, task.Frontmatter.Title)
            }

            // Create "blocks" link: blockerIssue blocks blockedIssue
            link := &jira.IssueLink{
                Type: jira.IssueLinkType{Name: "Blocks"},
                InwardIssue:  &jira.Issue{Key: blockedIssue},
                OutwardIssue: &jira.Issue{Key: blockerIssue},
            }

            if _, err := d.jiraClient.Issue.AddLink(link); err != nil {
                return fmt.Errorf("link %s -> %s: %w", blockerIssue, blockedIssue, err)
            }

            color.Green("✓ %s blocked by %s", blockedIssue, blockerIssue)
        }

        // Update status
        task.Frontmatter.SyncStatus = markdown.SyncStatusLinked
        if err := markdown.WriteTaskFile(task); err != nil {
            return fmt.Errorf("update file %s: %w", task.Path, err)
        }
    }

    return nil
}
```

---

## Task Extraction from Master Document

The `init` command will parse the master document and extract:

### 47 Tasks Identified

**Kubebuilder (4 tasks):**
- KB-1: Initialize Project and Repository (no deps)
- KB-2: Create Shared Type Definitions (deps: KB-1)
- KB-3: Create Shared Interfaces (deps: KB-2)
- KB-4: Create Mock Implementations (deps: KB-3)

**CRD (1 task):**
- CRD-1: Create DeploymentWatcher CRD (deps: KB-4)

**Controller (4 tasks):**
- CTRL-1: Create Basic Controller Scaffold (deps: KB-3, ERR-1)
- CTRL-2: Implement Namespace Filtering (deps: CTRL-1)
- CTRL-3: Implement Deployment Fetching (deps: CTRL-2)
- CTRL-4: Implement Requeue and Error Handling (deps: CTRL-3)

**RBAC (2 tasks):**
- RBAC-1: Define RBAC Roles (deps: KB-4)
- RBAC-2: Create ServiceAccount and Bindings (deps: RBAC-1)

**Error Detection (10 tasks):**
- ERR-1: Create Detector Stub Implementation (deps: KB-3)
- ERR-2: Implement Replica Failure Detection (deps: ERR-1)
- ERR-3: Implement Progress Deadline Detection (deps: ERR-1)
- ERR-4: Add Deployment-Level Error Aggregation (deps: ERR-2, ERR-3)
- ERR-5: Implement Pod Listing by Deployment (deps: ERR-1)
- ERR-6: Implement Container Waiting State Detection (deps: ERR-5)
- ERR-7: Implement Container Terminated State Detection (deps: ERR-5)
- ERR-8: Implement Event Listing and Filtering (deps: ERR-1)
- ERR-9: Implement Event-Based Error Detection (deps: ERR-8)
- ERR-10: Integrate All Detection Logic (deps: ERR-4, ERR-7, ERR-9)

**Metrics (12 tasks):**
- MET-1: Define Counter and Gauge Metrics (deps: KB-3)
- MET-2: Define Histogram Metrics and Register (deps: MET-1)
- MET-3: Create Metrics Documentation (deps: MET-2)
- MET-4: Create Collector Stub Implementation (deps: MET-1)
- MET-5: Implement RecordError and Counter Updates (deps: MET-4)
- MET-6: Implement SetActiveErrors and Gauge Updates (deps: MET-4)
- MET-7: Implement Replica and Duration Metrics (deps: MET-4)
- MET-8: Implement Metric Cleanup and Watched Deployments (deps: MET-4)
- MET-9: Implement Health Check Handlers (deps: KB-4)
- MET-10: Integrate Health Checks with Controller (deps: MET-9, CTRL-1)
- MET-11: Configure Structured Logging (deps: KB-4)
- MET-12: Add Reconciliation Logging (deps: MET-11, CTRL-1)

**Helm (14 tasks):**
- HELM-1: Create Chart Directory Structure (deps: KB-4)
- HELM-2: Create Helpers and Initial Values (deps: HELM-1)
- HELM-3: Create ServiceAccount and RBAC Templates (deps: HELM-2, RBAC-2)
- HELM-4: Create Deployment Template (deps: HELM-2)
- HELM-5: Create Service Template (deps: HELM-2)
- HELM-6: Create ServiceMonitor Template (deps: HELM-5)
- HELM-7: Create Optional Templates (deps: HELM-2)
- HELM-8: Define Core Values Configuration (deps: HELM-2)
- HELM-9: Define Resource and Security Values (deps: HELM-8)
- HELM-10: Create Values Schema (deps: HELM-9)
- HELM-11: Create Chart Documentation (deps: HELM-7, HELM-10)
- HELM-12: Create Helm Unit Tests (deps: HELM-7)
- HELM-13: Create CI Values and Testing (deps: HELM-12)
- HELM-14: Document Version Management (deps: HELM-11)

---

## Configuration

### Configuration Hierarchy (Viper)

Viper loads configuration in the following order (later sources override earlier):

1. **Default values** (hardcoded in the application)
2. **Config file** (`~/.jira-sync.yaml` or `--config` flag)
3. **Environment variables** (prefixed with `JIRA_`)
4. **Command-line flags**

### Config File (~/.jira-sync.yaml)

```yaml
jira:
  url: https://company.atlassian.net
  user: user@company.com
  # NOTE: Never put token in config file - use JIRA_TOKEN env var

defaults:
  issue_type: Task
  jira_state: Todo      # default jira-state for new tasks
  start_date_offset: 0  # days from creation
  end_date_offset: 7    # days from start (default: 7)

link_types:
  dependency: Blocks  # Jira link type name for dependencies
```

Note: The Jira project is specified per-task in the `jira-project` frontmatter field, not globally.

### Environment Variables

All environment variables are prefixed with `JIRA_`:

| Variable | Required | Description |
|----------|----------|-------------|
| `JIRA_TOKEN` | **Yes** | Jira API token (must be set via env var for security) |
| `JIRA_URL` | Yes* | Jira instance URL (e.g., `https://company.atlassian.net`) |
| `JIRA_USER` | Yes* | Jira username/email |
| `JIRA_DEFAULTS_ISSUE_TYPE` | No | Default issue type (default: `Task`) |
| `JIRA_DEFAULTS_JIRA_STATE` | No | Default Jira state for new tasks (default: `Todo`) |
| `JIRA_DEFAULTS_END_DATE_OFFSET` | No | Days to add for end date (default: `7`) |
| `JIRA_LINK_TYPES_DEPENDENCY` | No | Link type name (default: `Blocks`) |

*Can also be set in config file

Note: The Jira project is specified per-task in the `jira-project` frontmatter field, not via environment variable.

```bash
# Required
export JIRA_TOKEN="your-api-token"

# Required (or set in config file)
export JIRA_URL="https://company.atlassian.net"
export JIRA_USER="user@company.com"
```

### Security Note

**The `JIRA_TOKEN` must always be provided via environment variable**, never in the config file. This prevents accidental exposure of credentials in version control.

### Configuration Precedence Example

```bash
# Config file has: jira.url = "https://default.atlassian.net"
# Environment has: JIRA_URL = "https://override.atlassian.net"
# CLI flag: --jira-url "https://cli.atlassian.net"

# Result: "https://cli.atlassian.net" (CLI flag wins)
```

### Generating a Jira API Token

1. Log in to https://id.atlassian.com/manage-profile/security/api-tokens
2. Click "Create API token"
3. Give it a descriptive label (e.g., "jira-sync CLI")
4. Copy the token and set it as `JIRA_TOKEN`

```bash
export JIRA_TOKEN="ATATT3xFfGF0..."
```

---

## Error Handling

### Rollback Strategy

If ticket creation fails mid-batch:
1. All successfully created tickets remain (no automatic deletion)
2. Local files track sync-status (`created` vs `pending`)
3. Re-running `sync` only processes `pending` tickets
4. Manual cleanup instructions provided for partial failures

### Validation Before Creation

1. All task files must parse successfully
2. All dependencies must reference existing task IDs
3. No circular dependencies allowed
4. Parent epic must exist in Jira
5. User confirmation required before any Jira API calls

---

## Summary

| Approach | Effort | Flexibility | Error Handling | Maintenance |
|----------|--------|-------------|----------------|-------------|
| jira-cli (bash) | Low | Limited | Poor | Low |
| Custom Go CLI | Medium | High | Excellent | Medium |

**Recommendation: Build custom Go CLI**

The upfront investment (~2-3 days) pays off in:
1. Reliable batch operations
2. Two-phase dependency handling
3. Local file state tracking
4. Reusable for future projects
5. Better error recovery

---

## Next Steps

1. Create Go project structure in `jira/jira-sync/`
2. Implement markdown parser/writer
3. Implement Jira client wrapper
4. Build CLI commands (init, validate, create, link, sync, list)
5. Test with small batch of tickets
6. Run full batch creation for all 47 tasks
7. Create README.md documentation (see below)

---

## Task: Create README.md for Jira Task Files

Create a `jira/README.md` file that documents the Jira task file format. This README serves as context for Claude (and other AI assistants) to understand how to work with task files in future conversations.

### File: `jira/README.md`

```markdown
# Jira Task File Format

This directory contains markdown task files that sync with Jira tickets using the `jira-sync` CLI tool.

## Directory Structure

```
jira/
├── README.md           # This file - format documentation
├── plan.md             # Implementation plan for jira-sync tool
├── tasks/              # Individual task files (zettelkasten format)
│   ├── 20260116-103001.md
│   ├── 20260116-103002.md
│   └── ...
└── jira-sync/          # Go CLI tool source code
    ├── cmd/
    ├── internal/
    ├── main.go
    └── go.mod
```

## Task File Format

Each task is a markdown file with YAML frontmatter. Files use zettelkasten naming: `YYYYMMDD-HHMMSS.md`

### Example Task File

```yaml
---
title: "KB-1: Kubebuilder - Initialize Project and Repository"
jira-number: "GUARD-101"
jira-project: GUARD
jira-state: Todo
created-date: 2026-01-16
start-date: 2026-01-16
end-date: 2026-01-23
jira-url: "https://company.atlassian.net/browse/GUARD-101"
sync-status: linked
jira-parent: GUARD-100
sync-dependencies: []
jira-dependencies: []
content-hash: "a1b2c3d4e5f6..."
last-synced: "2026-01-21T10:30:00Z"
---

Task description goes here, including acceptance criteria.

This becomes the Jira ticket description field.
```

### Frontmatter Fields

| Field | Type | Description | Set By |
|-------|------|-------------|--------|
| `title` | string | Task ID and title (e.g., "KB-1: Title") | Manual/create |
| `jira-number` | string | Jira issue key (e.g., "GUARD-101") | jira-sync sync |
| `jira-project` | string | Jira project key (e.g., "GUARD") | Manual/create |
| `jira-state` | string | Jira ticket state (default: "Todo") | Manual/create, full-sync |
| `created-date` | date | Date file was created | jira-sync create |
| `start-date` | date | Date ticket created in Jira | jira-sync sync |
| `end-date` | date | start-date + 7 days | jira-sync sync |
| `jira-url` | string | Full URL to Jira issue | jira-sync sync |
| `sync-status` | string | Sync state: pending, created, linked | jira-sync sync |
| `jira-parent` | string | Parent epic/story key | Manual/create |
| `sync-dependencies` | array | Task IDs that must be created BEFORE this task (creation order) | Manual/create |
| `jira-dependencies` | array | Task IDs this task is blocked by (creates Jira links) | Manual/create |
| `content-hash` | string | SHA256 hash of entire file (for change detection) | jira-sync sync/full-sync |
| `last-synced` | datetime | ISO 8601 timestamp of last full-sync (for conflict detection) | jira-sync full-sync |

### Dependency Types

**`sync-dependencies`** - Controls ticket creation order
- Tasks listed here must be created in Jira before this task
- Uses topological sort to determine order
- Does NOT create links in Jira

**`jira-dependencies`** - Creates Jira "blocks" links
- Creates "X blocks Y" links in Jira after tickets are created
- Does NOT affect creation order

Most tasks will have the same values for both fields. Use `--deps` shorthand to set both.

### Sync Status Values

- `pending` - Task file exists, not yet created in Jira
- `created` - Ticket created in Jira, jira-dependencies not linked
- `linked` - Ticket created and all jira-dependencies linked

### Task ID Prefixes

Tasks use technology-area prefixes for easy identification:

| Prefix | Area | Example |
|--------|------|---------|
| KB- | Kubebuilder initialization | KB-1, KB-2, KB-3, KB-4 |
| CRD- | Custom Resource Definitions | CRD-1 |
| CTRL- | Controller implementation | CTRL-1, CTRL-2, CTRL-3, CTRL-4 |
| RBAC- | RBAC configuration | RBAC-1, RBAC-2 |
| ERR- | Error detection | ERR-1 through ERR-10 |
| MET- | Metrics & observability | MET-1 through MET-12 |
| HELM- | Helm chart | HELM-1 through HELM-14 |

### Dependencies Format

Dependencies are specified as arrays of task IDs:

```yaml
# Both sync and jira dependencies (most common case)
sync-dependencies:
  - KB-3
jira-dependencies:
  - KB-3
  - ERR-1
```

This means:
- KB-3 must be created in Jira BEFORE this task (sync-dependency)
- Both KB-3 and ERR-1 will have "blocks" links to this task in Jira (jira-dependencies)

## CLI Commands

### Create a task file (for Claude to use)

```bash
# Simple task
jira-sync create \
  --title "KB-1: Initialize Project" \
  --jira-parent GUARD-100 \
  --project GUARD \
  --description "Initialize the kubebuilder project"

# Task with both sync and jira deps set to same value (shorthand)
jira-sync create \
  --title "CTRL-1: Controller Scaffold" \
  --jira-parent GUARD-100 \
  --project GUARD \
  --description "Create the basic controller structure. Controller compiles and runs." \
  --deps "KB-3,ERR-1"

# Task with different sync and jira dependencies
jira-sync create \
  --title "ERR-5: Implement Pod Listing" \
  --jira-parent GUARD-100 \
  --project GUARD \
  --description "List pods by deployment." \
  --sync-deps "KB-3" \
  --jira-deps "KB-3,ERR-1"
```

### Sync with Jira (One-Way)

The `sync` command creates tickets and links dependencies. It is **one-way** (local → Jira).

```bash
# Set credentials
export JIRA_TOKEN="your-api-token"
export JIRA_URL="https://company.atlassian.net"
export JIRA_USER="user@company.com"

# Create tickets + link dependencies
jira-sync sync ./tasks/

# Dry run first
jira-sync sync ./tasks/ --dry-run

# Only update status from Jira
jira-sync sync ./tasks/ --status-only
```

### Full-Sync with Jira (Bidirectional)

The `full-sync` command keeps all fields synchronized between local files and Jira. It is **bidirectional**.

```bash
# Bidirectional sync (prompts on conflicts)
jira-sync full-sync ./tasks/

# Dry run to see what would change
jira-sync full-sync ./tasks/ --dry-run

# Local files always win on conflicts
jira-sync full-sync ./tasks/ --direction local

# Jira always wins on conflicts
jira-sync full-sync ./tasks/ --direction jira

# Only sync specific fields
jira-sync full-sync ./tasks/ --fields "title,description,state"
```

**Synchronized fields:**
- `title` ↔ Jira `summary`
- Description (body) ↔ Jira `description`
- `jira-state` ↔ Jira `status`
- `jira-parent` ↔ Jira `parent`

## Working with Task Files

### Creating a new task manually

1. Generate filename: `date +'%Y%m%d-%H%M%S'.md`
2. Copy frontmatter template from existing task
3. Fill in title, jira-parent, and dependencies
4. Set sync-status to `pending`
5. Write description (includes acceptance criteria)

### Updating an existing task

1. Edit the markdown body as needed
2. Do NOT manually change: jira-number, jira-url, sync-status
3. Dependencies can be updated before `sync` is run

### After Jira sync

After running `jira-sync sync`:
- `jira-number` will contain the Jira key (e.g., GUARD-101)
- `jira-url` will contain the full URL
- `sync-status` will be `linked`
- `start-date` and `end-date` will be set

## For AI Assistants (Claude)

When working with task files in this directory:

1. **Creating tasks**: Use the `jira-sync create` command with flags for each field
2. **Reading tasks**: Parse the YAML frontmatter to understand task metadata
3. **Finding dependencies**: Look at the `dependencies` array to understand task ordering
4. **Checking sync status**: Use `sync-status` field to know if task is synced with Jira
5. **Linking to Jira**: Use `jira-url` to reference the actual ticket
6. **Task identification**: Extract task ID from `title` (e.g., "KB-1" from "KB-1: Title")

### Creating tasks (preferred method for Claude)

Use the `jira-sync create` command to generate properly formatted task files:

```bash
# Basic task
jira-sync create \
  --title "TASK-ID: Task Title" \
  --jira-parent "PARENT-KEY" \
  --project "PROJECT-KEY" \
  --description "Task description"

# Full task with dependencies
jira-sync create \
  --title "ERR-5: Implement Pod Listing" \
  --jira-parent "GUARD-100" \
  --project "GUARD" \
  --description "Implement the logic to list all Pods belonging to a Deployment. Unit tests with fake client required." \
  --dependencies "ERR-1" \
  --output "./jira/tasks/"
```

### Common queries

- "What tasks depend on KB-3?" → Search for files with `KB-3` in dependencies
- "What's the Jira ticket for ERR-5?" → Read `jira-number` from that task file
- "What tasks are pending?" → Find files with `sync-status: pending`
- "Show the critical path" → Follow dependency chain from tasks with no dependencies
- "Create a new task" → Use `jira-sync create` command with appropriate flags
```

### Purpose of this README

This README ensures that:

1. **Future Claude sessions** can understand the task file format without re-explanation
2. **Team members** have documentation on how to work with task files
3. **The jira-sync tool** has usage documentation alongside the code
4. **Consistency** is maintained across all task files

---

## Implementation Plan (TDD)

This section provides a detailed implementation plan using Test-Driven Development (RED → GREEN → REFACTOR).

### Architecture Principles

1. **Interface-driven design** - All external dependencies behind interfaces for easy mocking
2. **Separation of concerns** - File operations, Jira API, and business logic are separate
3. **Dependency injection** - All dependencies passed in, not created internally
4. **Small, focused functions** - Each function does one thing well

### Core Interfaces

```go
// internal/ports/ports.go

// TaskRepository handles reading/writing task files (file system abstraction)
type TaskRepository interface {
    // ReadTask reads a single task file and parses frontmatter
    ReadTask(path string) (*domain.TaskFile, error)

    // WriteTask writes a task file with frontmatter and description
    WriteTask(task *domain.TaskFile) error

    // ListTasks returns all task files in a directory
    ListTasks(dir string) ([]*domain.TaskFile, error)

    // GenerateFilename creates a zettelkasten filename
    GenerateFilename() string
}

// JiraClient handles all Jira API operations
type JiraClient interface {
    // CreateIssue creates a new issue and returns the created issue with key
    CreateIssue(ctx context.Context, req CreateIssueRequest) (*Issue, error)

    // UpdateIssue updates an existing issue
    UpdateIssue(ctx context.Context, key string, req UpdateIssueRequest) error

    // CreateLink creates a dependency link between two issues
    CreateLink(ctx context.Context, inward, outward, linkType string) error

    // GetIssue fetches an issue by key
    GetIssue(ctx context.Context, key string) (*Issue, error)
}

// HashComputer computes content hashes for change detection
type HashComputer interface {
    // ComputeHash returns SHA256 hash of task content
    ComputeHash(task *domain.TaskFile) string
}

// UserPrompter handles user interaction (confirmations, etc.)
type UserPrompter interface {
    // Confirm asks user yes/no question, returns true if yes
    Confirm(message string) bool
}
```

---

### Phase 1: Project Setup & Domain Types

#### 1.1 Initialize Go Module

```bash
mkdir -p jira/jira-sync
cd jira/jira-sync
go mod init github.com/curtbushko/jira-sync
```

#### 1.2 Create Domain Types (No tests needed - pure data structures)

- [ ] Create `internal/domain/task.go` with TaskFile, Frontmatter structs
- [ ] Create `internal/domain/constants.go` with SyncStatus constants
- [ ] Create `internal/domain/errors.go` with custom error types

---

### Phase 2: Task Repository (File Operations)

#### 2.1 Task Parser - Parse Frontmatter

**RED: Write failing test**
```go
// internal/adapters/filesystem/parser_test.go
func TestParseTask_ValidFile(t *testing.T) {
    content := `---
title: "KB-1: Test Task"
jira-number: ""
sync-status: pending
jira-parent: GUARD-100
dependencies: []
content-hash: ""
---

Task description here.`

    parser := NewParser()
    task, err := parser.Parse("test.md", content)

    require.NoError(t, err)
    assert.Equal(t, "KB-1: Test Task", task.Frontmatter.Title)
    assert.Equal(t, "pending", task.Frontmatter.SyncStatus)
    assert.Equal(t, "Task description here.", task.Description)
}

func TestParseTask_InvalidFrontmatter(t *testing.T) {
    content := `not valid frontmatter`

    parser := NewParser()
    _, err := parser.Parse("test.md", content)

    assert.Error(t, err)
}

func TestParseTask_MissingRequiredFields(t *testing.T) {
    content := `---
title: ""
---
Description`

    parser := NewParser()
    _, err := parser.Parse("test.md", content)

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "title is required")
}
```

**GREEN: Implement parser**
- [ ] Implement `internal/adapters/filesystem/parser.go`
- [ ] Split frontmatter from body using `---` delimiter
- [ ] Parse YAML frontmatter with `gopkg.in/yaml.v3`
- [ ] Validate required fields

**REFACTOR: Clean up**
- [ ] Extract validation to separate function
- [ ] Add detailed error messages with line numbers

#### 2.2 Task Writer - Write Frontmatter

**RED: Write failing test**
```go
func TestWriteTask_ValidTask(t *testing.T) {
    task := &domain.TaskFile{
        Path: "test.md",
        Frontmatter: domain.Frontmatter{
            Title:      "KB-1: Test",
            SyncStatus: "pending",
            Parent:     "GUARD-100",
        },
        Description: "Task description",
    }

    writer := NewWriter()
    content, err := writer.Marshal(task)

    require.NoError(t, err)
    assert.Contains(t, content, "title: \"KB-1: Test\"")
    assert.Contains(t, content, "sync-status: pending")
    assert.Contains(t, content, "Task description")
}
```

**GREEN: Implement writer**
- [ ] Implement `internal/adapters/filesystem/writer.go`
- [ ] Marshal frontmatter to YAML
- [ ] Combine with description body

**REFACTOR**
- [ ] Ensure consistent field ordering in output

#### 2.3 Task Repository - Full Implementation

**RED: Write failing tests**
```go
func TestTaskRepository_ReadTask(t *testing.T) {
    // Setup: create temp file with valid content
    tmpDir := t.TempDir()
    content := `---
title: "KB-1: Test"
sync-status: pending
jira-parent: GUARD-100
dependencies: []
content-hash: ""
---

Description`
    path := filepath.Join(tmpDir, "20260116-120000.md")
    os.WriteFile(path, []byte(content), 0644)

    repo := NewFileTaskRepository()
    task, err := repo.ReadTask(path)

    require.NoError(t, err)
    assert.Equal(t, "KB-1: Test", task.Frontmatter.Title)
}

func TestTaskRepository_WriteTask(t *testing.T) {
    tmpDir := t.TempDir()
    task := &domain.TaskFile{
        Path: filepath.Join(tmpDir, "test.md"),
        Frontmatter: domain.Frontmatter{
            Title:      "KB-1: Test",
            SyncStatus: "pending",
            Parent:     "GUARD-100",
        },
        Description: "Test description",
    }

    repo := NewFileTaskRepository()
    err := repo.WriteTask(task)

    require.NoError(t, err)

    // Read back and verify
    readTask, err := repo.ReadTask(task.Path)
    require.NoError(t, err)
    assert.Equal(t, task.Frontmatter.Title, readTask.Frontmatter.Title)
}

func TestTaskRepository_ListTasks(t *testing.T) {
    tmpDir := t.TempDir()
    // Create 3 task files
    for i := 0; i < 3; i++ {
        content := fmt.Sprintf(`---
title: "TASK-%d"
sync-status: pending
jira-parent: GUARD-100
dependencies: []
content-hash: ""
---

Description %d`, i, i)
        path := filepath.Join(tmpDir, fmt.Sprintf("2026011%d-120000.md", i))
        os.WriteFile(path, []byte(content), 0644)
    }

    repo := NewFileTaskRepository()
    tasks, err := repo.ListTasks(tmpDir)

    require.NoError(t, err)
    assert.Len(t, tasks, 3)
}

func TestTaskRepository_GenerateFilename(t *testing.T) {
    repo := NewFileTaskRepository()

    filename := repo.GenerateFilename()

    // Should match pattern YYYYMMDD-HHMMSS.md
    assert.Regexp(t, `^\d{8}-\d{6}\.md$`, filename)
}
```

**GREEN: Implement repository**
- [ ] Implement `internal/adapters/filesystem/repository.go`
- [ ] Implement TaskRepository interface
- [ ] Use parser/writer internally

**REFACTOR**
- [ ] Add file locking for concurrent access
- [ ] Improve error messages with file paths

---

### Phase 3: Hash Computer

#### 3.1 Content Hash Computation

**RED: Write failing tests**
```go
func TestHashComputer_ComputeHash(t *testing.T) {
    task := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{
            Title:        "KB-1: Test",
            Parent:       "GUARD-100",
            Dependencies: []string{"KB-0"},
        },
        Description: "Test description",
    }

    hasher := NewSHA256HashComputer()
    hash := hasher.ComputeHash(task)

    assert.NotEmpty(t, hash)
    assert.Len(t, hash, 64) // SHA256 hex = 64 chars
}

func TestHashComputer_SameContentSameHash(t *testing.T) {
    task1 := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{Title: "Test", Parent: "P"},
        Description: "Desc",
    }
    task2 := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{Title: "Test", Parent: "P"},
        Description: "Desc",
    }

    hasher := NewSHA256HashComputer()

    assert.Equal(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_DifferentContentDifferentHash(t *testing.T) {
    task1 := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{Title: "Test1", Parent: "P"},
        Description: "Desc",
    }
    task2 := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{Title: "Test2", Parent: "P"},
        Description: "Desc",
    }

    hasher := NewSHA256HashComputer()

    assert.NotEqual(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}
```

**GREEN: Implement hash computer**
- [ ] Implement `internal/adapters/hash/sha256.go`
- [ ] Hash title + jira-parent + dependencies + description
- [ ] Return hex-encoded SHA256

**REFACTOR**
- [ ] Ensure deterministic ordering of dependencies in hash

---

### Phase 4: Jira Client (Mock First)

#### 4.1 Create Mock Jira Client

**No RED needed - this is test infrastructure**
```go
// internal/adapters/jira/mock_client.go
type MockJiraClient struct {
    CreateIssueFunc func(ctx context.Context, req CreateIssueRequest) (*Issue, error)
    UpdateIssueFunc func(ctx context.Context, key string, req UpdateIssueRequest) error
    CreateLinkFunc  func(ctx context.Context, inward, outward, linkType string) error
    GetIssueFunc    func(ctx context.Context, key string) (*Issue, error)

    // Call tracking
    CreateIssueCalls []CreateIssueRequest
    UpdateIssueCalls []UpdateIssueCall
    CreateLinkCalls  []CreateLinkCall
}

func (m *MockJiraClient) CreateIssue(ctx context.Context, req CreateIssueRequest) (*Issue, error) {
    m.CreateIssueCalls = append(m.CreateIssueCalls, req)
    if m.CreateIssueFunc != nil {
        return m.CreateIssueFunc(ctx, req)
    }
    return &Issue{Key: "MOCK-" + strconv.Itoa(len(m.CreateIssueCalls))}, nil
}
// ... implement other methods
```

- [ ] Implement `internal/adapters/jira/mock_client.go`

#### 4.2 Real Jira Client

**RED: Write integration test (skipped in CI)**
```go
// +build integration

func TestJiraClient_CreateIssue_Integration(t *testing.T) {
    if os.Getenv("JIRA_TOKEN") == "" {
        t.Skip("JIRA_TOKEN not set")
    }

    client := NewJiraClient(
        os.Getenv("JIRA_URL"),
        os.Getenv("JIRA_USER"),
        os.Getenv("JIRA_TOKEN"),
    )

    issue, err := client.CreateIssue(context.Background(), CreateIssueRequest{
        Project:     "TEST",
        Summary:     "Integration Test Issue",
        Description: "Created by integration test",
        IssueType:   "Task",
    })

    require.NoError(t, err)
    assert.NotEmpty(t, issue.Key)

    // Cleanup: delete the issue
    // ...
}
```

**GREEN: Implement real client**
- [ ] Implement `internal/adapters/jira/client.go`
- [ ] Use `github.com/andygrunwald/go-jira`
- [ ] Implement CreateIssue, UpdateIssue, CreateLink, GetIssue

**REFACTOR**
- [ ] Add retry logic for transient failures
- [ ] Add rate limiting

---

### Phase 4.5: Topological Sort (for sync-dependencies)

#### 4.5.1 Topological Sort Implementation

**RED: Write failing tests**
```go
func TestTopologicalSort_SimpleChain(t *testing.T) {
    // KB-1 -> KB-2 -> KB-3
    tasks := []*domain.TaskFile{
        {Frontmatter: domain.Frontmatter{Title: "KB-3: Third", SyncDependencies: []string{"KB-2"}}},
        {Frontmatter: domain.Frontmatter{Title: "KB-1: First", SyncDependencies: []string{}}},
        {Frontmatter: domain.Frontmatter{Title: "KB-2: Second", SyncDependencies: []string{"KB-1"}}},
    }

    sorted, err := sync.TopologicalSort(tasks, tasks)

    require.NoError(t, err)
    assert.Equal(t, "KB-1", sorted[0].TaskID())
    assert.Equal(t, "KB-2", sorted[1].TaskID())
    assert.Equal(t, "KB-3", sorted[2].TaskID())
}

func TestTopologicalSort_MultipleDependencies(t *testing.T) {
    // CTRL-1 depends on both KB-3 and ERR-1
    tasks := []*domain.TaskFile{
        {Frontmatter: domain.Frontmatter{Title: "CTRL-1: Controller", SyncDependencies: []string{"KB-3", "ERR-1"}}},
        {Frontmatter: domain.Frontmatter{Title: "KB-3: Types", SyncDependencies: []string{}}},
        {Frontmatter: domain.Frontmatter{Title: "ERR-1: Detector", SyncDependencies: []string{}}},
    }

    sorted, err := sync.TopologicalSort(tasks, tasks)

    require.NoError(t, err)
    // CTRL-1 must be last
    assert.Equal(t, "CTRL-1", sorted[2].TaskID())
    // KB-3 and ERR-1 can be in either order (both have no deps)
}

func TestTopologicalSort_CircularDependency(t *testing.T) {
    // KB-1 -> KB-2 -> KB-1 (circular!)
    tasks := []*domain.TaskFile{
        {Frontmatter: domain.Frontmatter{Title: "KB-1: First", SyncDependencies: []string{"KB-2"}}},
        {Frontmatter: domain.Frontmatter{Title: "KB-2: Second", SyncDependencies: []string{"KB-1"}}},
    }

    _, err := sync.TopologicalSort(tasks, tasks)

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "circular")
}

func TestTopologicalSort_DependencyAlreadyCreated(t *testing.T) {
    // KB-2 depends on KB-1, but KB-1 is already created (not in pending list)
    pending := []*domain.TaskFile{
        {Frontmatter: domain.Frontmatter{Title: "KB-2: Second", SyncDependencies: []string{"KB-1"}}},
    }
    allTasks := []*domain.TaskFile{
        {Frontmatter: domain.Frontmatter{Title: "KB-1: First", SyncStatus: "created", JiraNumber: "GUARD-101"}},
        {Frontmatter: domain.Frontmatter{Title: "KB-2: Second", SyncDependencies: []string{"KB-1"}}},
    }

    sorted, err := sync.TopologicalSort(pending, allTasks)

    require.NoError(t, err)
    // KB-2 can be created immediately since KB-1 already exists
    assert.Len(t, sorted, 1)
    assert.Equal(t, "KB-2", sorted[0].TaskID())
}

func TestTopologicalSort_NoDependencies(t *testing.T) {
    tasks := []*domain.TaskFile{
        {Frontmatter: domain.Frontmatter{Title: "KB-1: First", SyncDependencies: []string{}}},
        {Frontmatter: domain.Frontmatter{Title: "KB-2: Second", SyncDependencies: []string{}}},
        {Frontmatter: domain.Frontmatter{Title: "KB-3: Third", SyncDependencies: []string{}}},
    }

    sorted, err := sync.TopologicalSort(tasks, tasks)

    require.NoError(t, err)
    assert.Len(t, sorted, 3)
    // Order doesn't matter when there are no dependencies
}
```

**GREEN: Implement topological sort**
- [ ] Implement `internal/sync/toposort.go`
- [ ] Use Kahn's algorithm for topological sort
- [ ] Detect circular dependencies
- [ ] Handle partially created dependencies (some tasks already in Jira)

**REFACTOR**
- [ ] Add descriptive error messages showing the cycle
- [ ] Optimize for large task sets

---

### Phase 5: Sync Service (Business Logic)

#### 5.1 Task Categorization

**RED: Write failing tests**
```go
func TestSyncService_CategorizesTasks(t *testing.T) {
    tasks := []*domain.TaskFile{
        {Frontmatter: domain.Frontmatter{SyncStatus: "pending"}},
        {Frontmatter: domain.Frontmatter{SyncStatus: "created"}},
        {Frontmatter: domain.Frontmatter{SyncStatus: "linked", ContentHash: "abc"}, Description: "same"},
        {Frontmatter: domain.Frontmatter{SyncStatus: "linked", ContentHash: "old"}, Description: "changed"},
    }

    mockHasher := &MockHashComputer{
        ComputeHashFunc: func(t *domain.TaskFile) string {
            if t.Description == "same" {
                return "abc"
            }
            return "new"
        },
    }

    svc := NewSyncService(nil, nil, mockHasher, nil)
    result := svc.CategorizeTasks(tasks)

    assert.Len(t, result.Pending, 1)
    assert.Len(t, result.Created, 1)
    assert.Len(t, result.Linked, 1)
    assert.Len(t, result.NeedsUpdate, 1)
}
```

**GREEN: Implement categorization**
- [ ] Implement `internal/application/sync/service.go`
- [ ] Categorize tasks by sync-status and content-hash

#### 5.2 Create Tickets Flow

**RED: Write failing tests**
```go
func TestSyncService_CreateTickets(t *testing.T) {
    pendingTask := &domain.TaskFile{
        Path: "/tasks/test.md",
        Frontmatter: domain.Frontmatter{
            Title:      "KB-1: Test",
            SyncStatus: "pending",
            Parent:     "GUARD-100",
        },
        Description: "Test description",
    }

    mockJira := &MockJiraClient{
        CreateIssueFunc: func(ctx context.Context, req CreateIssueRequest) (*Issue, error) {
            return &Issue{Key: "GUARD-101", Self: "https://jira/GUARD-101"}, nil
        },
    }
    mockRepo := &MockTaskRepository{}
    mockHasher := &MockHashComputer{
        ComputeHashFunc: func(t *domain.TaskFile) string { return "newhash" },
    }

    svc := NewSyncService(mockRepo, mockJira, mockHasher, nil)
    err := svc.CreateTickets(context.Background(), []*domain.TaskFile{pendingTask}, "GUARD")

    require.NoError(t, err)

    // Verify Jira was called
    assert.Len(t, mockJira.CreateIssueCalls, 1)
    assert.Equal(t, "KB-1: Test", mockJira.CreateIssueCalls[0].Summary)

    // Verify task was updated
    assert.Len(t, mockRepo.WriteTaskCalls, 1)
    written := mockRepo.WriteTaskCalls[0]
    assert.Equal(t, "GUARD-101", written.Frontmatter.JiraNumber)
    assert.Equal(t, "created", written.Frontmatter.SyncStatus)
    assert.Equal(t, "newhash", written.Frontmatter.ContentHash)
}

func TestSyncService_CreateTickets_JiraError(t *testing.T) {
    pendingTask := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{Title: "Test", SyncStatus: "pending"},
    }

    mockJira := &MockJiraClient{
        CreateIssueFunc: func(ctx context.Context, req CreateIssueRequest) (*Issue, error) {
            return nil, errors.New("jira error")
        },
    }

    svc := NewSyncService(nil, mockJira, nil, nil)
    err := svc.CreateTickets(context.Background(), []*domain.TaskFile{pendingTask}, "GUARD")

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "jira error")
}
```

**GREEN: Implement create tickets**
- [ ] Implement CreateTickets method
- [ ] Call Jira API, update task file, save

#### 5.3 Link Jira Dependencies Flow

**RED: Write failing tests**
```go
func TestSyncService_LinkJiraDependencies(t *testing.T) {
    tasks := []*domain.TaskFile{
        {
            Frontmatter: domain.Frontmatter{
                Title:            "KB-1: First",
                JiraNumber:       "GUARD-101",
                SyncStatus:       "created",
                JiraDependencies: []string{},
            },
        },
        {
            Frontmatter: domain.Frontmatter{
                Title:            "KB-2: Second",
                JiraNumber:       "GUARD-102",
                SyncStatus:       "created",
                JiraDependencies: []string{"KB-1"},
            },
        },
    }

    mockJira := &MockJiraClient{}
    mockRepo := &MockTaskRepository{}

    svc := NewSyncService(mockRepo, mockJira, nil, nil)
    err := svc.LinkJiraDependencies(context.Background(), tasks)

    require.NoError(t, err)

    // Verify link was created: GUARD-102 blocked by GUARD-101
    assert.Len(t, mockJira.CreateLinkCalls, 1)
    assert.Equal(t, "GUARD-102", mockJira.CreateLinkCalls[0].Inward)
    assert.Equal(t, "GUARD-101", mockJira.CreateLinkCalls[0].Outward)

    // Verify tasks updated to linked
    assert.Len(t, mockRepo.WriteTaskCalls, 2)
}

func TestSyncService_LinkJiraDependencies_ResolvesTaskIDs(t *testing.T) {
    // Test that "KB-1" in jira-dependencies resolves to "GUARD-101"
    // ...
}

func TestSyncService_LinkJiraDependencies_IgnoresSyncDeps(t *testing.T) {
    // Verify that sync-dependencies do NOT create Jira links
    tasks := []*domain.TaskFile{
        {
            Frontmatter: domain.Frontmatter{
                Title:            "KB-1: First",
                JiraNumber:       "GUARD-101",
                SyncStatus:       "created",
                SyncDependencies: []string{},
                JiraDependencies: []string{},
            },
        },
        {
            Frontmatter: domain.Frontmatter{
                Title:            "KB-2: Second",
                JiraNumber:       "GUARD-102",
                SyncStatus:       "created",
                SyncDependencies: []string{"KB-1"}, // Only sync dep, no jira dep
                JiraDependencies: []string{},
            },
        },
    }

    mockJira := &MockJiraClient{}
    mockRepo := &MockTaskRepository{}

    svc := NewSyncService(mockRepo, mockJira, nil, nil)
    err := svc.LinkJiraDependencies(context.Background(), tasks)

    require.NoError(t, err)

    // No Jira links should be created (sync-deps don't create links)
    assert.Len(t, mockJira.CreateLinkCalls, 0)
}
```

**GREEN: Implement link jira-dependencies**
- [ ] Build task ID → Jira key map
- [ ] Create links for each jira-dependency (NOT sync-dependency)
- [ ] Update sync-status to linked

#### 5.4 Update Modified Tickets Flow

**RED: Write failing tests**
```go
func TestSyncService_UpdateModified(t *testing.T) {
    modifiedTask := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{
            Title:       "KB-1: Updated Title",
            JiraNumber:  "GUARD-101",
            SyncStatus:  "linked",
            ContentHash: "oldhash",
        },
        Description: "Updated description",
    }

    mockJira := &MockJiraClient{}
    mockRepo := &MockTaskRepository{}
    mockHasher := &MockHashComputer{
        ComputeHashFunc: func(t *domain.TaskFile) string { return "newhash" },
    }

    svc := NewSyncService(mockRepo, mockJira, mockHasher, nil)
    err := svc.UpdateModified(context.Background(), []*domain.TaskFile{modifiedTask})

    require.NoError(t, err)

    // Verify Jira update was called
    assert.Len(t, mockJira.UpdateIssueCalls, 1)
    assert.Equal(t, "GUARD-101", mockJira.UpdateIssueCalls[0].Key)
    assert.Equal(t, "KB-1: Updated Title", mockJira.UpdateIssueCalls[0].Summary)

    // Verify hash was updated
    assert.Equal(t, "newhash", mockRepo.WriteTaskCalls[0].Frontmatter.ContentHash)
}
```

**GREEN: Implement update modified**
- [ ] Implement UpdateModified method
- [ ] Call Jira update API
- [ ] Recompute and save content hash

---

### Phase 6: CLI Commands (Cobra)

#### 6.1 Create Command

**RED: Write failing tests**
```go
func TestCreateCommand_CreatesTaskFile(t *testing.T) {
    tmpDir := t.TempDir()

    mockRepo := &MockTaskRepository{
        GenerateFilenameFunc: func() string { return "20260116-120000.md" },
    }

    cmd := NewCreateCommand(mockRepo)
    cmd.SetArgs([]string{
        "--title", "KB-1: Test Task",
        "--jira-parent", "GUARD-100",
        "--description", "Test description",
        "--output", tmpDir,
    })

    err := cmd.Execute()

    require.NoError(t, err)
    assert.Len(t, mockRepo.WriteTaskCalls, 1)

    written := mockRepo.WriteTaskCalls[0]
    assert.Equal(t, "KB-1: Test Task", written.Frontmatter.Title)
    assert.Equal(t, "pending", written.Frontmatter.SyncStatus)
}

func TestCreateCommand_RequiresTitle(t *testing.T) {
    cmd := NewCreateCommand(nil)
    cmd.SetArgs([]string{
        "--jira-parent", "GUARD-100",
        "--description", "Test",
    })

    err := cmd.Execute()

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "title")
}
```

**GREEN: Implement create command**
- [ ] Implement `cmd/create.go`
- [ ] Parse flags
- [ ] Create TaskFile and save via repository

#### 6.2 Sync Command

**RED: Write failing tests**
```go
func TestSyncCommand_DryRun(t *testing.T) {
    mockRepo := &MockTaskRepository{
        ListTasksFunc: func(dir string) ([]*domain.TaskFile, error) {
            return []*domain.TaskFile{
                {Frontmatter: domain.Frontmatter{SyncStatus: "pending", Title: "Test", JiraProject: "GUARD", JiraState: "Todo"}},
            }, nil
        },
    }
    mockJira := &MockJiraClient{}

    cmd := NewSyncCommand(mockRepo, mockJira, nil, nil)
    cmd.SetArgs([]string{"./tasks", "--dry-run"})

    var output bytes.Buffer
    cmd.SetOut(&output)

    err := cmd.Execute()

    require.NoError(t, err)
    assert.Contains(t, output.String(), "1 pending")
    assert.Empty(t, mockJira.CreateIssueCalls) // Dry run = no API calls
}

func TestSyncCommand_WithConfirmation(t *testing.T) {
    mockPrompter := &MockUserPrompter{ConfirmFunc: func(msg string) bool { return true }}
    // ...
}
```

**GREEN: Implement sync command**
- [ ] Implement `cmd/sync.go`
- [ ] Wire up SyncService
- [ ] Add confirmation prompts
- [ ] Add --dry-run flag

---

### Phase 7: Configuration (Viper)

#### 7.1 Config Loading

**RED: Write failing tests**
```go
func TestConfig_LoadsFromFile(t *testing.T) {
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, ".jira-sync.yaml")
    configContent := `
jira:
  url: https://test.atlassian.net
  user: test@test.com
defaults:
  issue_type: Story
  jira_state: In Progress
`
    os.WriteFile(configPath, []byte(configContent), 0644)

    cfg, err := LoadConfig(configPath)

    require.NoError(t, err)
    assert.Equal(t, "https://test.atlassian.net", cfg.Jira.URL)
    assert.Equal(t, "Story", cfg.Defaults.IssueType)
    assert.Equal(t, "In Progress", cfg.Defaults.JiraState)
}

func TestConfig_EnvironmentOverrides(t *testing.T) {
    os.Setenv("JIRA_URL", "https://env.atlassian.net")
    defer os.Unsetenv("JIRA_URL")

    cfg, err := LoadConfig("")

    require.NoError(t, err)
    assert.Equal(t, "https://env.atlassian.net", cfg.Jira.URL)
}

func TestConfig_RequiresToken(t *testing.T) {
    os.Unsetenv("JIRA_TOKEN")

    _, err := LoadConfig("")

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "JIRA_TOKEN")
}
```

**GREEN: Implement config loading**
- [ ] Implement `internal/config/config.go`
- [ ] Use Viper for config hierarchy

---

### Phase 8: Integration & E2E Tests

#### 8.1 End-to-End Create Flow

```go
func TestE2E_CreateAndSync(t *testing.T) {
    if os.Getenv("JIRA_TOKEN") == "" {
        t.Skip("JIRA_TOKEN not set")
    }

    tmpDir := t.TempDir()

    // 1. Create a task file
    createCmd := NewCreateCommand(NewFileTaskRepository())
    createCmd.SetArgs([]string{
        "--title", "E2E-1: Test Task",
        "--jira-parent", os.Getenv("TEST_EPIC"),
        "--project", "TEST",
        "--description", "E2E test task",
        "--output", tmpDir,
    })
    require.NoError(t, createCmd.Execute())

    // 2. Sync to Jira (project is read from task file's jira-project field)
    syncCmd := NewSyncCommand(/* real dependencies */)
    syncCmd.SetArgs([]string{tmpDir, "--yes"})
    require.NoError(t, syncCmd.Execute())

    // 3. Verify task file was updated
    tasks, _ := NewFileTaskRepository().ListTasks(tmpDir)
    assert.Equal(t, "created", tasks[0].Frontmatter.SyncStatus)
    assert.NotEmpty(t, tasks[0].Frontmatter.JiraNumber)

    // Cleanup...
}
```

- [ ] Write E2E tests (skipped in CI without credentials)

---

### Implementation Checklist Summary

#### Phase 1: Project Setup
- [ ] 1.1 Initialize Go module
- [ ] 1.2 Create domain types (with SyncDependencies and JiraDependencies)

#### Phase 2: Task Repository
- [ ] 2.1 Parser - RED/GREEN/REFACTOR
- [ ] 2.2 Writer - RED/GREEN/REFACTOR
- [ ] 2.3 Repository - RED/GREEN/REFACTOR

#### Phase 3: Hash Computer
- [ ] 3.1 SHA256 hash - RED/GREEN/REFACTOR (hash includes jira-deps, excludes sync-deps)

#### Phase 4: Jira Client
- [ ] 4.1 Mock client
- [ ] 4.2 Real client - RED/GREEN/REFACTOR

#### Phase 4.5: Topological Sort
- [ ] 4.5.1 Topological sort for sync-dependencies - RED/GREEN/REFACTOR

#### Phase 5: Sync Service
- [ ] 5.1 Task categorization - RED/GREEN/REFACTOR
- [ ] 5.2 Create tickets (in topological order) - RED/GREEN/REFACTOR
- [ ] 5.3 Link jira-dependencies (NOT sync-dependencies) - RED/GREEN/REFACTOR
- [ ] 5.4 Update modified - RED/GREEN/REFACTOR

#### Phase 6: CLI Commands
- [ ] 6.1 Create command (with --sync-deps, --jira-deps, --deps flags) - RED/GREEN/REFACTOR
- [ ] 6.2 Sync command (with topological sort) - RED/GREEN/REFACTOR

#### Phase 7: Configuration
- [ ] 7.1 Config loading - RED/GREEN/REFACTOR

#### Phase 8: Integration Tests
- [ ] 8.1 E2E tests

---

### Dependency Graph

```
Phase 1 (Domain Types)
    ↓
Phase 2 (Task Repository) ←──────────┐
    ↓                                │
Phase 3 (Hash Computer)              │
    ↓                                │
Phase 4 (Jira Client - Mock) ────────┤
    ↓                                │
Phase 4.5 (Topological Sort) ────────┤
    ↓                                │
Phase 5 (Sync Service) ──────────────┤
    ↓                                │
Phase 6 (CLI Commands) ──────────────┘
    ↓
Phase 7 (Configuration)
    ↓
Phase 8 (Integration Tests)
```

Phases 2, 3, 4 (Mock), and 4.5 can be developed in parallel.
Phase 5 requires 2, 3, 4 (Mock), and 4.5.
Phase 6 requires 5.
Phase 4 (Real) can be developed alongside Phase 5.

---

### Phase 9: Wiki URL Format for Dependencies

Dependencies use wiki-style markdown links for navigation: `[Task Title](filename.md)`

#### 9.1 Wiki Link Parser

**RED: Write failing tests**
```go
// internal/adapters/filesystem/wikilink_test.go
func TestParseWikiLink_ValidLink(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        wantTaskID string
        wantFile   string
    }{
        {
            name:       "full wiki link",
            input:      "[KB-1: Initialize Project](20260116-103001.md)",
            wantTaskID: "KB-1",
            wantFile:   "20260116-103001.md",
        },
        {
            name:       "wiki link with complex title",
            input:      "[ERR-2: Handle Pod Failures - Container State](20260116-103002.md)",
            wantTaskID: "ERR-2",
            wantFile:   "20260116-103002.md",
        },
        {
            name:       "legacy plain task ID",
            input:      "KB-1",
            wantTaskID: "KB-1",
            wantFile:   "",
        },
        {
            name:       "legacy with whitespace",
            input:      "  KB-1  ",
            wantTaskID: "KB-1",
            wantFile:   "",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            taskID, filename := ParseWikiLink(tt.input)
            assert.Equal(t, tt.wantTaskID, taskID)
            assert.Equal(t, tt.wantFile, filename)
        })
    }
}

func TestParseWikiLink_InvalidLinks(t *testing.T) {
    tests := []struct {
        name  string
        input string
    }{
        {"empty string", ""},
        {"malformed link missing paren", "[KB-1: Title](file.md"},
        {"malformed link missing bracket", "KB-1: Title](file.md)"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            taskID, _ := ParseWikiLink(tt.input)
            // Should return empty or handle gracefully
            if tt.input == "" {
                assert.Equal(t, "", taskID)
            }
        })
    }
}
```

**GREEN: Implement parser**
```go
// internal/adapters/filesystem/wikilink.go
package filesystem

import (
    "regexp"
    "strings"
)

// wikiLinkRegex matches wiki-style links: [Title](filename.md)
var wikiLinkRegex = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+\.md)\)`)

// ParseWikiLink extracts task ID and filename from a wiki-style link
// Supports both formats:
//   - Wiki link: "[KB-1: Initialize Project](20260116-103001.md)" -> ("KB-1", "20260116-103001.md")
//   - Legacy: "KB-1" -> ("KB-1", "")
func ParseWikiLink(link string) (taskID string, filename string) {
    matches := wikiLinkRegex.FindStringSubmatch(link)
    if len(matches) == 3 {
        title := matches[1]
        filename = matches[2]
        parts := strings.SplitN(title, ":", 2)
        taskID = strings.TrimSpace(parts[0])
        return taskID, filename
    }
    return strings.TrimSpace(link), ""
}
```

- [ ] Implement `internal/adapters/filesystem/wikilink.go`
- [ ] Add regex for wiki link parsing
- [ ] Handle legacy plain task ID format
- [ ] Add edge case handling for malformed links

**REFACTOR**
- [ ] Extract regex to package-level constant
- [ ] Add validation for filename format (zettelkasten pattern)

#### 9.2 Wiki Link Formatter

**RED: Write failing tests**
```go
func TestFormatWikiLink(t *testing.T) {
    tests := []struct {
        taskID   string
        title    string
        filename string
        want     string
    }{
        {
            taskID:   "KB-1",
            title:    "Initialize Project",
            filename: "20260116-103001.md",
            want:     "[KB-1: Initialize Project](20260116-103001.md)",
        },
        {
            taskID:   "ERR-5",
            title:    "Pod Listing",
            filename: "20260116-103005.md",
            want:     "[ERR-5: Pod Listing](20260116-103005.md)",
        },
    }

    for _, tt := range tests {
        t.Run(tt.taskID, func(t *testing.T) {
            got := FormatWikiLink(tt.taskID, tt.title, tt.filename)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

**GREEN: Implement formatter**
- [ ] Implement `FormatWikiLink()` function
- [ ] Combine taskID, title, and filename into proper format

#### 9.3 Dependency ID Extraction Methods

**RED: Write failing tests**
```go
func TestTaskFile_GetSyncDependencyIDs(t *testing.T) {
    task := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{
            SyncDependencies: []string{
                "[KB-1: Initialize Project](20260116-103001.md)",
                "[KB-2: Create Types](20260116-103002.md)",
                "KB-3", // legacy format
            },
        },
    }

    ids := task.GetSyncDependencyIDs()

    assert.Equal(t, []string{"KB-1", "KB-2", "KB-3"}, ids)
}

func TestTaskFile_GetJiraDependencyIDs(t *testing.T) {
    task := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{
            JiraDependencies: []string{
                "[ERR-1: Error Handler](20260116-104001.md)",
            },
        },
    }

    ids := task.GetJiraDependencyIDs()

    assert.Equal(t, []string{"ERR-1"}, ids)
}

func TestTaskFile_GetDependencyIDs_Empty(t *testing.T) {
    task := &domain.TaskFile{}

    syncIDs := task.GetSyncDependencyIDs()
    jiraIDs := task.GetJiraDependencyIDs()

    assert.Empty(t, syncIDs)
    assert.Empty(t, jiraIDs)
}
```

**GREEN: Implement extraction methods**
- [ ] Add `GetSyncDependencyIDs()` method to TaskFile
- [ ] Add `GetJiraDependencyIDs()` method to TaskFile
- [ ] Use `ParseWikiLink()` internally

**REFACTOR**
- [ ] Extract common logic to helper function
- [ ] Add caching if performance becomes an issue

---

### Phase 10: Frontmatter Migration (Legacy Support)

Handle older task files that are missing frontmatter fields.

#### 10.1 Migration Detection

**RED: Write failing tests**
```go
// internal/adapters/filesystem/migration_test.go
func TestMigrateFrontmatter_MissingFields(t *testing.T) {
    task := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{
            Title:      "KB-1: Test Task",
            JiraParent: "GUARD-100",
            // All other fields missing
        },
    }

    migrated := task.MigrateFrontmatter()

    assert.True(t, migrated, "should return true when fields were added")
    assert.Equal(t, "Todo", task.Frontmatter.JiraState)
    assert.Equal(t, "pending", task.Frontmatter.SyncStatus)
    assert.NotNil(t, task.Frontmatter.SyncDependencies)
    assert.NotNil(t, task.Frontmatter.JiraDependencies)
    assert.Empty(t, task.Frontmatter.SyncDependencies)
    assert.Empty(t, task.Frontmatter.JiraDependencies)
}

func TestMigrateFrontmatter_AllFieldsPresent(t *testing.T) {
    task := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{
            Title:            "KB-1: Test Task",
            JiraNumber:       "GUARD-123",
            JiraProject:      "GUARD",
            JiraState:        "In Progress",
            SyncStatus:       "linked",
            JiraParent:       "GUARD-100",
            SyncDependencies: []string{"KB-0"},
            JiraDependencies: []string{"KB-0"},
            ContentHash:      "abc123",
            LastSynced:       "2026-01-20T10:00:00Z",
        },
    }

    migrated := task.MigrateFrontmatter()

    assert.False(t, migrated, "should return false when no migration needed")
    assert.Equal(t, "In Progress", task.Frontmatter.JiraState) // unchanged
    assert.Equal(t, "linked", task.Frontmatter.SyncStatus)     // unchanged
}

func TestMigrateFrontmatter_PartialFields(t *testing.T) {
    task := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{
            Title:      "KB-1: Test",
            JiraState:  "Done", // already set
            SyncStatus: "",     // missing
        },
    }

    migrated := task.MigrateFrontmatter()

    assert.True(t, migrated)
    assert.Equal(t, "Done", task.Frontmatter.JiraState)   // preserved
    assert.Equal(t, "pending", task.Frontmatter.SyncStatus) // set default
}
```

**GREEN: Implement migration**
```go
// internal/domain/task.go (add method)

// MigrateFrontmatter adds missing fields with default values
// Returns true if any fields were migrated
func (t *TaskFile) MigrateFrontmatter() bool {
    migrated := false

    if t.Frontmatter.JiraState == "" {
        t.Frontmatter.JiraState = "Todo"
        migrated = true
    }
    if t.Frontmatter.SyncStatus == "" {
        t.Frontmatter.SyncStatus = SyncStatusPending
        migrated = true
    }
    if t.Frontmatter.SyncDependencies == nil {
        t.Frontmatter.SyncDependencies = []string{}
        migrated = true
    }
    if t.Frontmatter.JiraDependencies == nil {
        t.Frontmatter.JiraDependencies = []string{}
        migrated = true
    }

    return migrated
}
```

- [ ] Add `MigrateFrontmatter()` method to TaskFile
- [ ] Set defaults for all missing fields
- [ ] Return boolean indicating if migration occurred

**REFACTOR**
- [ ] Extract default values to constants
- [ ] Add logging for migrated fields

#### 10.2 Parser Integration with Migration

**RED: Write failing tests**
```go
func TestParseTaskFile_AutoMigration(t *testing.T) {
    // Old format file missing new fields
    content := `---
title: "KB-1: Old Task"
jira-parent: GUARD-100
---

Old task description.`

    tmpDir := t.TempDir()
    path := filepath.Join(tmpDir, "old-task.md")
    os.WriteFile(path, []byte(content), 0644)

    repo := NewFileTaskRepository()
    task, err := repo.ReadTask(path)

    require.NoError(t, err)
    assert.Equal(t, "Todo", task.Frontmatter.JiraState)
    assert.Equal(t, "pending", task.Frontmatter.SyncStatus)
    assert.NotNil(t, task.Frontmatter.SyncDependencies)
}
```

**GREEN: Integrate migration into parser**
- [ ] Call `MigrateFrontmatter()` after parsing
- [ ] Migration happens in memory, not persisted until WriteTask

#### 10.3 Migrate Command

**RED: Write failing tests**
```go
// cmd/migrate_test.go
func TestMigrateCommand_DryRun(t *testing.T) {
    // Setup temp directory with old-format task files
    tmpDir := t.TempDir()
    oldContent := `---
title: "KB-1: Old Task"
jira-parent: GUARD-100
---

Description.`
    os.WriteFile(filepath.Join(tmpDir, "old.md"), []byte(oldContent), 0644)

    // Run migrate command with --dry-run
    cmd := NewMigrateCmd()
    cmd.SetArgs([]string{tmpDir, "--dry-run"})

    var stdout bytes.Buffer
    cmd.SetOut(&stdout)
    err := cmd.Execute()

    require.NoError(t, err)
    assert.Contains(t, stdout.String(), "Would migrate")

    // Verify file was NOT modified
    content, _ := os.ReadFile(filepath.Join(tmpDir, "old.md"))
    assert.NotContains(t, string(content), "jira-state")
}

func TestMigrateCommand_ActualMigration(t *testing.T) {
    tmpDir := t.TempDir()
    oldContent := `---
title: "KB-1: Old Task"
jira-parent: GUARD-100
---

Description.`
    path := filepath.Join(tmpDir, "old.md")
    os.WriteFile(path, []byte(oldContent), 0644)

    cmd := NewMigrateCmd()
    cmd.SetArgs([]string{tmpDir})
    err := cmd.Execute()

    require.NoError(t, err)

    // Verify file WAS modified
    content, _ := os.ReadFile(path)
    assert.Contains(t, string(content), "jira-state: Todo")
    assert.Contains(t, string(content), "sync-status: pending")
}

func TestMigrateCommand_DefaultProject(t *testing.T) {
    tmpDir := t.TempDir()
    oldContent := `---
title: "KB-1: Task Missing Project"
jira-parent: GUARD-100
---

Description.`
    path := filepath.Join(tmpDir, "task.md")
    os.WriteFile(path, []byte(oldContent), 0644)

    cmd := NewMigrateCmd()
    cmd.SetArgs([]string{tmpDir, "--default-project", "MYPROJ"})
    err := cmd.Execute()

    require.NoError(t, err)

    content, _ := os.ReadFile(path)
    assert.Contains(t, string(content), "jira-project: MYPROJ")
}
```

**GREEN: Implement migrate command**
- [ ] Create `cmd/migrate.go`
- [ ] Walk directory for .md files
- [ ] Parse each file and check for migration needs
- [ ] Write back migrated files (unless --dry-run)
- [ ] Support --default-project flag

**REFACTOR**
- [ ] Add progress indicator for large directories
- [ ] Add summary statistics at end

---

### Phase 11: Full-Sync Command (Bidirectional)

Bidirectional synchronization between local files and Jira.

#### 11.1 Change Detection - Local vs Jira

**RED: Write failing tests**
```go
// internal/services/fullsync/detector_test.go
func TestDetectChanges_LocalOnlyChanged(t *testing.T) {
    task := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{
            Title:       "KB-1: Test",
            JiraNumber:  "GUARD-123",
            ContentHash: "oldhash",
            LastSynced:  "2026-01-15T10:00:00Z",
        },
        Description: "Updated description", // Changed locally
    }

    jiraIssue := &ports.Issue{
        Key:         "GUARD-123",
        Summary:     "KB-1: Test",
        Description: "Original description",
        Updated:     time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC), // Before last sync
    }

    detector := NewChangeDetector(mockHashComputer)
    result := detector.Detect(task, jiraIssue)

    assert.Equal(t, ChangeTypeLocalToJira, result.Type)
    assert.Contains(t, result.Fields, "description")
}

func TestDetectChanges_JiraOnlyChanged(t *testing.T) {
    task := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{
            Title:       "KB-1: Test",
            JiraNumber:  "GUARD-123",
            ContentHash: "currenthash", // Matches current content
            LastSynced:  "2026-01-15T10:00:00Z",
        },
        Description: "Original description",
    }

    jiraIssue := &ports.Issue{
        Key:         "GUARD-123",
        Summary:     "KB-1: Updated Title", // Changed in Jira
        Description: "Original description",
        Updated:     time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), // After last sync
    }

    detector := NewChangeDetector(mockHashComputer)
    result := detector.Detect(task, jiraIssue)

    assert.Equal(t, ChangeTypeJiraToLocal, result.Type)
    assert.Contains(t, result.Fields, "title")
}

func TestDetectChanges_BothChanged_Conflict(t *testing.T) {
    task := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{
            Title:       "KB-1: Local Title",
            JiraNumber:  "GUARD-123",
            ContentHash: "oldhash", // Different from current
            LastSynced:  "2026-01-15T10:00:00Z",
        },
        Description: "Local description",
    }

    jiraIssue := &ports.Issue{
        Key:         "GUARD-123",
        Summary:     "KB-1: Jira Title",
        Description: "Jira description",
        Updated:     time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), // After last sync
    }

    detector := NewChangeDetector(mockHashComputer)
    result := detector.Detect(task, jiraIssue)

    assert.Equal(t, ChangeTypeConflict, result.Type)
    assert.NotEmpty(t, result.Conflicts)
}

func TestDetectChanges_NoChanges(t *testing.T) {
    task := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{
            Title:       "KB-1: Test",
            JiraNumber:  "GUARD-123",
            ContentHash: "currenthash",
            LastSynced:  "2026-01-15T10:00:00Z",
        },
        Description: "Same description",
    }

    jiraIssue := &ports.Issue{
        Key:         "GUARD-123",
        Summary:     "KB-1: Test",
        Description: "Same description",
        Updated:     time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC), // Before last sync
    }

    detector := NewChangeDetector(mockHashComputer)
    result := detector.Detect(task, jiraIssue)

    assert.Equal(t, ChangeTypeNone, result.Type)
}
```

**GREEN: Implement change detector**
- [ ] Create `internal/services/fullsync/detector.go`
- [ ] Implement local change detection (hash comparison)
- [ ] Implement Jira change detection (timestamp comparison)
- [ ] Detect conflicts when both changed

**REFACTOR**
- [ ] Extract field comparison to separate methods
- [ ] Add detailed conflict information

#### 11.2 Field Comparison

**RED: Write failing tests**
```go
func TestCompareFields_Title(t *testing.T) {
    local := "KB-1: Local Title"
    jira := "KB-1: Jira Title"

    diff := CompareField("title", local, jira)

    assert.True(t, diff.IsDifferent)
    assert.Equal(t, local, diff.LocalValue)
    assert.Equal(t, jira, diff.JiraValue)
}

func TestCompareFields_Description(t *testing.T) {
    local := "Description\nwith newlines"
    jira := "Description\nwith newlines"

    diff := CompareField("description", local, jira)

    assert.False(t, diff.IsDifferent)
}

func TestCompareFields_State(t *testing.T) {
    local := "Todo"
    jira := "In Progress"

    diff := CompareField("state", local, jira)

    assert.True(t, diff.IsDifferent)
}
```

**GREEN: Implement field comparison**
- [ ] Create field comparison functions
- [ ] Handle whitespace normalization for descriptions
- [ ] Support all syncable fields (title, description, state, parent)

#### 11.3 Full-Sync Service

**RED: Write failing tests**
```go
// internal/services/fullsync/service_test.go
func TestFullSyncService_Compare(t *testing.T) {
    mockRepo := &MockTaskRepository{}
    mockJira := &MockJiraClient{}

    tasks := []*domain.TaskFile{
        {
            Frontmatter: domain.Frontmatter{
                Title:      "KB-1: Task 1",
                JiraNumber: "GUARD-101",
            },
        },
        {
            Frontmatter: domain.Frontmatter{
                Title:      "KB-2: Task 2",
                JiraNumber: "GUARD-102",
            },
        },
    }

    mockJira.On("GetIssue", mock.Anything, "GUARD-101").Return(&ports.Issue{
        Key:     "GUARD-101",
        Summary: "KB-1: Task 1",
    }, nil)
    mockJira.On("GetIssue", mock.Anything, "GUARD-102").Return(&ports.Issue{
        Key:     "GUARD-102",
        Summary: "KB-2: Updated", // Changed in Jira
    }, nil)

    service := NewFullSyncService(mockRepo, mockJira)
    results, err := service.Compare(context.Background(), tasks)

    require.NoError(t, err)
    assert.Len(t, results.JiraToLocal, 1)
    assert.Equal(t, "KB-2", results.JiraToLocal[0].TaskID)
}

func TestFullSyncService_Apply_LocalToJira(t *testing.T) {
    mockRepo := &MockTaskRepository{}
    mockJira := &MockJiraClient{}

    change := &Change{
        Task:     &domain.TaskFile{...},
        TaskID:   "KB-1",
        Field:    "description",
        NewValue: "Updated description",
    }

    mockJira.On("UpdateIssue", mock.Anything, "GUARD-101", mock.Anything).Return(nil)
    mockRepo.On("WriteTask", mock.Anything).Return(nil)

    service := NewFullSyncService(mockRepo, mockJira)
    err := service.ApplyChange(context.Background(), change, DirectionLocalToJira)

    require.NoError(t, err)
    mockJira.AssertCalled(t, "UpdateIssue", mock.Anything, "GUARD-101", mock.Anything)
}

func TestFullSyncService_Apply_JiraToLocal(t *testing.T) {
    mockRepo := &MockTaskRepository{}
    mockJira := &MockJiraClient{}

    task := &domain.TaskFile{
        Frontmatter: domain.Frontmatter{
            Title:      "KB-1: Old Title",
            JiraNumber: "GUARD-101",
        },
    }

    change := &Change{
        Task:     task,
        TaskID:   "KB-1",
        Field:    "title",
        NewValue: "KB-1: New Title",
    }

    mockRepo.On("WriteTask", mock.Anything).Return(nil)

    service := NewFullSyncService(mockRepo, mockJira)
    err := service.ApplyChange(context.Background(), change, DirectionJiraToLocal)

    require.NoError(t, err)
    assert.Equal(t, "KB-1: New Title", task.Frontmatter.Title)
    mockRepo.AssertCalled(t, "WriteTask", task)
}
```

**GREEN: Implement full-sync service**
- [ ] Create `internal/services/fullsync/service.go`
- [ ] Implement `Compare()` method
- [ ] Implement `Apply()` method
- [ ] Handle conflict resolution (local wins, jira wins, ask)

**REFACTOR**
- [ ] Add batch operations for efficiency
- [ ] Add transaction-like behavior with rollback

#### 11.4 Full-Sync Command

**RED: Write failing tests**
```go
// cmd/fullsync_test.go
func TestFullSyncCommand_DryRun(t *testing.T) {
    // Setup mocks and test data
    cmd := NewFullSyncCmd(mockService)
    cmd.SetArgs([]string{"./tasks", "--dry-run"})

    var stdout bytes.Buffer
    cmd.SetOut(&stdout)
    err := cmd.Execute()

    require.NoError(t, err)
    assert.Contains(t, stdout.String(), "Dry run")
    // Verify no actual changes were made
}

func TestFullSyncCommand_DirectionLocal(t *testing.T) {
    cmd := NewFullSyncCmd(mockService)
    cmd.SetArgs([]string{"./tasks", "--direction", "local"})
    err := cmd.Execute()

    require.NoError(t, err)
    // Verify conflicts resolved with local winning
}

func TestFullSyncCommand_DirectionJira(t *testing.T) {
    cmd := NewFullSyncCmd(mockService)
    cmd.SetArgs([]string{"./tasks", "--direction", "jira"})
    err := cmd.Execute()

    require.NoError(t, err)
    // Verify conflicts resolved with jira winning
}

func TestFullSyncCommand_FieldFilter(t *testing.T) {
    cmd := NewFullSyncCmd(mockService)
    cmd.SetArgs([]string{"./tasks", "--fields", "title,description"})
    err := cmd.Execute()

    require.NoError(t, err)
    // Verify only specified fields were synced
}
```

**GREEN: Implement full-sync command**
- [ ] Create `cmd/fullsync.go`
- [ ] Add --dry-run flag
- [ ] Add --direction flag (local, jira, ask)
- [ ] Add --fields flag for field filtering
- [ ] Add --yes flag to skip confirmation

**REFACTOR**
- [ ] Add progress bar for large sync operations
- [ ] Add colored output for changes

---

### Phase 12: Status Transitions (Jira Workflow)

Handle Jira status/workflow transitions properly.

#### 12.1 Status Transition Service

**RED: Write failing tests**
```go
// internal/services/jira/transition_test.go
func TestTransitionIssue_ValidTransition(t *testing.T) {
    mockClient := &MockJiraClient{}
    mockClient.On("GetTransitions", mock.Anything, "GUARD-123").Return([]ports.Transition{
        {ID: "21", Name: "In Progress"},
        {ID: "31", Name: "Done"},
    }, nil)
    mockClient.On("DoTransition", mock.Anything, "GUARD-123", "21").Return(nil)

    service := NewTransitionService(mockClient)
    err := service.TransitionTo(context.Background(), "GUARD-123", "In Progress")

    require.NoError(t, err)
    mockClient.AssertCalled(t, "DoTransition", mock.Anything, "GUARD-123", "21")
}

func TestTransitionIssue_InvalidTransition(t *testing.T) {
    mockClient := &MockJiraClient{}
    mockClient.On("GetTransitions", mock.Anything, "GUARD-123").Return([]ports.Transition{
        {ID: "21", Name: "In Progress"},
    }, nil)

    service := NewTransitionService(mockClient)
    err := service.TransitionTo(context.Background(), "GUARD-123", "Done")

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "transition to 'Done' not available")
}

func TestTransitionIssue_AlreadyInState(t *testing.T) {
    mockClient := &MockJiraClient{}
    mockClient.On("GetIssue", mock.Anything, "GUARD-123").Return(&ports.Issue{
        Status: "Done",
    }, nil)

    service := NewTransitionService(mockClient)
    err := service.TransitionTo(context.Background(), "GUARD-123", "Done")

    require.NoError(t, err)
    // No transition should be attempted
    mockClient.AssertNotCalled(t, "DoTransition")
}
```

**GREEN: Implement transition service**
- [ ] Create `internal/services/jira/transition.go`
- [ ] Fetch available transitions for issue
- [ ] Find transition by target state name
- [ ] Execute transition

**REFACTOR**
- [ ] Cache transition mappings
- [ ] Add helpful error messages with available transitions

---

### Updated Implementation Checklist Summary

#### Phase 1: Project Setup
- [ ] 1.1 Initialize Go module
- [ ] 1.2 Create domain types

#### Phase 2: Task Repository
- [ ] 2.1 Parser - RED/GREEN/REFACTOR
- [ ] 2.2 Writer - RED/GREEN/REFACTOR
- [ ] 2.3 Repository - RED/GREEN/REFACTOR

#### Phase 3: Hash Computer
- [ ] 3.1 SHA256 hash - RED/GREEN/REFACTOR

#### Phase 4: Jira Client
- [ ] 4.1 Mock client
- [ ] 4.2 Real client - RED/GREEN/REFACTOR

#### Phase 4.5: Topological Sort
- [ ] 4.5.1 Topological sort - RED/GREEN/REFACTOR

#### Phase 5: Sync Service
- [ ] 5.1 Task categorization - RED/GREEN/REFACTOR
- [ ] 5.2 Create tickets - RED/GREEN/REFACTOR
- [ ] 5.3 Link dependencies - RED/GREEN/REFACTOR
- [ ] 5.4 Update modified - RED/GREEN/REFACTOR

#### Phase 6: CLI Commands (Basic)
- [ ] 6.1 Create command - RED/GREEN/REFACTOR
- [ ] 6.2 Sync command - RED/GREEN/REFACTOR

#### Phase 7: Configuration
- [ ] 7.1 Config loading - RED/GREEN/REFACTOR

#### Phase 8: Integration Tests
- [ ] 8.1 E2E tests

#### Phase 9: Wiki URL Format
- [ ] 9.1 Wiki link parser - RED/GREEN/REFACTOR
- [ ] 9.2 Wiki link formatter - RED/GREEN/REFACTOR
- [ ] 9.3 Dependency ID extraction - RED/GREEN/REFACTOR

#### Phase 10: Frontmatter Migration
- [ ] 10.1 Migration detection - RED/GREEN/REFACTOR
- [ ] 10.2 Parser integration - RED/GREEN/REFACTOR
- [ ] 10.3 Migrate command - RED/GREEN/REFACTOR

#### Phase 11: Full-Sync (Bidirectional)
- [ ] 11.1 Change detection - RED/GREEN/REFACTOR
- [ ] 11.2 Field comparison - RED/GREEN/REFACTOR
- [ ] 11.3 Full-sync service - RED/GREEN/REFACTOR
- [ ] 11.4 Full-sync command - RED/GREEN/REFACTOR

#### Phase 12: Status Transitions
- [ ] 12.1 Transition service - RED/GREEN/REFACTOR

---

### Updated Dependency Graph

```
Phase 1 (Domain Types)
    ↓
Phase 2 (Task Repository) ←───────────────────────┐
    ↓                                              │
Phase 3 (Hash Computer)                            │
    ↓                                              │
Phase 4 (Jira Client - Mock) ──────────────────────┤
    ↓                                              │
Phase 4.5 (Topological Sort) ──────────────────────┤
    ↓                                              │
Phase 9 (Wiki URL Format) ─────────────────────────┤
    ↓                                              │
Phase 10 (Frontmatter Migration) ──────────────────┤
    ↓                                              │
Phase 5 (Sync Service) ────────────────────────────┤
    ↓                                              │
Phase 6 (CLI Commands - Basic) ────────────────────┤
    ↓                                              │
Phase 11 (Full-Sync Service) ──────────────────────┤
    ↓                                              │
Phase 12 (Status Transitions) ─────────────────────┘
    ↓
Phase 7 (Configuration)
    ↓
Phase 8 (Integration Tests)
```

**Parallel Development Opportunities:**
- Phases 2, 3, 4 (Mock), 4.5, 9, 10 can be developed in parallel
- Phase 5 requires 2, 3, 4 (Mock), 4.5, 9
- Phase 6 requires 5
- Phase 11 requires 5, 10
- Phase 12 requires 4 (Real)
- Phases 7 and 8 come last
