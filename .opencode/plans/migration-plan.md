# OpenCode Migration Plan

## Files to Create

### 1. `opencode.json` (repo root)

```json
{
  "$schema": "https://opencode.ai/config.json",
  "instructions": ["AGENTS.md", ".github/copilot-instructions.md"],
  "skills": {
    "paths": [".opencode/skills", ".github/skills"]
  },
  "default_agent": "team-leader",
  "mcp": {
    "github-projects": {
      "type": "remote",
      "url": "https://api.githubcopilot.com/mcp/",
      "enabled": true,
      "headers": {
        "Authorization": "Bearer {env:GITHUB_PERSONAL_ACCESS_TOKEN}",
        "X-MCP-Toolsets": "projects,issues,pull_requests,repos",
        "X-MCP-Readonly": "false"
      }
    }
  }
}
```

### 2. Agents (`.opencode/agents/*.md`)

All 24 agents from `.github/agents/` need to be migrated. The frontmatter conversion:

**GitHub Copilot format:**
```markdown
---
name: agent-name
description: Description text
tools: ["read", "search", "edit", "execute", "agent"]
---
```

**OpenCode format:**
```markdown
---
description: Description text
mode: all
---
```

Key changes:
- Remove `name:` (filename provides it)
- Remove `tools:` (not an opencode field)
- Add `mode: all` (or `mode: subagent` for speckit workflow agents)
- Body content stays identical

#### Agent list and recommended modes:

| Agent File | Mode | Notes |
|---|---|---|
| `speckit.analyze.md` | subagent | Read-only analysis |
| `speckit.checklist.md` | subagent | Validation workflow |
| `speckit.clarify.md` | subagent | Clarification workflow |
| `speckit.constitution.md` | subagent | Setup workflow |
| `speckit.implement.md` | all | Full implementation agent |
| `speckit.plan.md` | all | Planning agent |
| `speckit.specify.md` | all | Spec creation agent |
| `speckit.tasks.md` | subagent | Task generation |
| `speckit.taskstoissues.md` | subagent | Issue sync |
| `speckit.git.commit.md` | subagent | Git operations |
| `speckit.git.feature.md` | subagent | Git operations |
| `speckit.git.initialize.md` | subagent | Git operations |
| `speckit.git.remote.md` | subagent | Git operations |
| `speckit.git.validate.md` | subagent | Git operations |
| `senior-golang-developer.md` | all | Primary backend agent |
| `senior-flutter-mobile-engineer.md` | all | Primary mobile agent |
| `architect.md` | all | Architecture agent |
| `tech-lead.md` | all | Code review agent |
| `team-leader.md` | primary | Default coordination agent |
| `project-manager.md` | all | PM agent |
| `database-optimizer.md` | subagent | Specialized |
| `devops-automator.md` | subagent | Specialized |
| `incident-response-commander.md` | subagent | Specialized |
| `sre.md` | subagent | Specialized |

### 3. Skills (`.opencode/skills/*/SKILL.md`)

Copy the following from `.github/skills/` to `.opencode/skills/`:
- `clean-code-guard/SKILL.md`
- `test-guard/SKILL.md`
- `docs-guard/SKILL.md`
- `flutter-ui-ux.md` → `flutter-ui-ux/SKILL.md` (needs folder restructuring)
- `prd-refinement/` → full directory
- `steno-mode/` → full directory
- `architecture-review/` → full directory

The `opencode.json` already references `.github/skills` in `skills.paths`, so you can either:
- **Option A**: Keep skills in `.github/skills/` (already referenced, no copy needed)
- **Option B**: Copy to `.opencode/skills/` for opencode-native location

### 4. Commands (`.opencode/commands/*.md`)

Convert from `.github/prompts/*.prompt.md` format:

**GitHub Copilot prompt format:**
```markdown
---
agent: speckit.specify
---
```
(This just references the agent — the actual logic is in the agent file)

**OpenCode command format:**
```markdown
---
description: Create or update the feature specification from a natural language feature description.
agent: speckit.specify
---

$ARGUMENTS
```

The command body should reference the agent's workflow. Since speckit agents already contain their full workflow in the agent file, the command can be minimal:

```markdown
---
description: Create or update the feature specification from a natural language feature description.
agent: speckit.specify
---

Execute the speckit.specify agent workflow with the following input:

$ARGUMENTS
```

#### Command list (from prompts):

| Prompt File | Command File | Agent |
|---|---|---|
| `speckit.specify.prompt.md` | `speckit.specify.md` | speckit.specify |
| `speckit.clarify.prompt.md` | `speckit.clarify.md` | speckit.clarify |
| `speckit.checklist.prompt.md` | `speckit.checklist.md` | speckit.checklist |
| `speckit.plan.prompt.md` | `speckit.plan.md` | speckit.plan |
| `speckit.tasks.prompt.md` | `speckit.tasks.md` | speckit.tasks |
| `speckit.implement.prompt.md` | `speckit.implement.md` | speckit.implement |
| `speckit.analyze.prompt.md` | `speckit.analyze.md` | speckit.analyze |
| `speckit.taskstoissues.prompt.md` | `speckit.taskstoissues.md` | speckit.taskstoissues |
| `speckit.constitution.prompt.md` | `speckit.constitution.md` | speckit.constitution |
| `speckit.git.commit.prompt.md` | `speckit.git.commit.md` | speckit.git.commit |
| `speckit.git.feature.prompt.md` | `speckit.git.feature.md` | speckit.git.feature |
| `speckit.git.initialize.prompt.md` | `speckit.git.initialize.md` | speckit.git.initialize |
| `speckit.git.remote.prompt.md` | `speckit.git.remote.md` | speckit.git.remote |
| `speckit.git.validate.prompt.md` | `speckit.git.validate.md` | speckit.git.validate |

## Usage After Migration

Once all files are created:

1. **Quit and restart opencode** for config changes to take effect
2. **Commands**: Type `/speckit.specify add user authentication` etc.
3. **Agents**: Switch agents via the agent selector or reference with `@agent-name`
4. **Skills**: Skills auto-load; invoke with `$skill-name` (e.g., `$clean-code-guard`)
5. **MCP**: GitHub Projects MCP will be available if `GITHUB_PERSONAL_ACCESS_TOKEN` is set

## Extension Hooks

The `.specify/extensions.yml` hook system works the same way — agents check for hooks before/after each phase. No changes needed.

## PowerShell Scripts

The `.specify/scripts/powershell/` scripts are referenced by agents and work unchanged. No migration needed.
