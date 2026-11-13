### Added

- `ship skill install`: ship-it's Agent Skill, a `SKILL.md` that tells coding agents how to deliver work with ship-it and what to leave to a human, written in `.agents/skills` (Codex, Cursor, VS Code) and `.claude/skills` (Claude Code). `ship mcp install` writes it too (`--no-skill` leaves it out), `ship undo` and `ship uninstall` remove it, and a `SKILL.md` of your own is never replaced (#50).
