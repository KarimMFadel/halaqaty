# Client agent configuration

Halaqaty keeps client-specific agent metadata in the format each client loads. These profiles are adapters to the role briefs, not separate policy authorities:

- GitHub Copilot: `.github/agents/*.agent.md`
- OpenCode: `.opencode/agents/*.md`
- Codex: `.codex/agents/*.toml`; profiles direct Codex to the matching Copilot role brief, as required by `AGENTS.md`.
- Kimi Code CLI: `.kimi-code/agents/*.md`; profiles direct Kimi to the matching OpenCode role brief. Do not confuse Kimi Code CLI configuration with another Kimi product.

Spec-Kit workflow agents remain in their existing client-specific directories and continue to own the feature lifecycle. The role agents provide focused domain advice and implementation within approved scope. Client metadata and tool controls are intentionally platform-specific; compare role prompt content and governing instructions when reviewing changes. No generator or universal canonical agent format is assumed.

Kimi's Team Leader profile is allowed to delegate to the defined project roles; use only the smallest relevant set under the workflow harness. Tech Lead is a read-only reviewer in Copilot, OpenCode, Codex, and Kimi. Parent-session permissions still govern execution.

Project quality skills remain in .github/skills/ for Copilot and .opencode/skills/ for OpenCode. Codex and Kimi should follow the harness fallback: read the relevant project SKILL.md and apply it manually unless that client exposes the skill as callable. Do not add duplicate .agents/skills/ wrappers while OpenCode also discovers that shared directory; duplicate skill names can conflict.

Third-party skill repositories are discovery sources, not trusted project requirements. Review source, license, dependencies, tool access, and Halaqaty fit before importing; do not add a mandatory scanner or CI gate without evidence.

## Validation

When client releases are available, check each client's own schema and profile discovery behavior. A file-format check alone does not prove runtime discovery. For Codex, custom project profiles are standalone TOML files. For Kimi Code CLI, custom project profiles are Markdown files with YAML frontmatter; project `AGENTS.md` remains an instruction file, not an agent profile.