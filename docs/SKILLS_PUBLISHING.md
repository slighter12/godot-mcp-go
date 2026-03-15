# Skills Publishing Guide

This repository keeps the Go MCP server and the companion agent skills in the
same Git repository.

That is the current intended model.

## What `npx` / `bunx` actually means

When users run commands such as:

```bash
npx skills add owner/repo --skill skill-name
bunx skills add owner/repo --skill skill-name
```

`npm` / `bun` is only being used to execute the Skills CLI. The installed
payload is still a GitHub-hosted skill repository, not an npm package
published to the npm registry.

For this repository, that means:

- you do **not** need to add `package.json` just to make skills installable
- you do **not** need to change the MCP protocol surface
- you **do** need a stable `skills/` layout that the Skills CLI can discover

## Current repository model

Current layout:

```text
godot-mcp-go/
├── skills/
│   ├── policy-godot/
│   │   └── SKILL.md
│   └── godot-game-dev-workflow/
│       └── SKILL.md
├── README.md
└── ...
```

This repository acts as both:

- the source repository for the Go MCP server
- the installation source for the companion skills

That combined model is acceptable here because the skills are tightly coupled
to this repository's `godot.*` tool contract and are authored as companion
artifacts for consumers of this server.

## Supported install pattern

Install directly from this repository:

```bash
npx skills add https://github.com/slighter12/godot-mcp-go --skill policy-godot
bunx skills add https://github.com/slighter12/godot-mcp-go --skill godot-game-dev-workflow
```

The GitHub repository is the install source. There is no separate npm package
required for skill installation.

Pinned install pattern after a release tag exists:

See [`README.md`](../README.md) for the current pinned install examples and the
active repository tag once one is published.

## Versioning strategy

This repository should use one shared repository version line for both:

- the Go MCP server
- the companion skills published from `skills/`

Use a Git tag in the `v0.x.x` format as the shared repository reference point
for both the server and the bundled skills. Future tags can continue on the
pre-`1.0` line.

Recommended stance:

- keep the active repository version and pinned tag examples in
  [`README.md`](../README.md)
- use one tag for the whole repository snapshot
- do not create separate skill-only version numbers at this stage
- bump the repository tag whenever skill behavior or server behavior changes in a way worth referencing

Why this is the default:

- users need one stable point to reference when reporting issues
- skill content and `godot.*` tool behavior evolve together in this repository
- a shared tag keeps rollback and compatibility discussions simple
- installs are only reproducible when the repo URL is pinned to a tag or commit SHA

When communicating changes, describe them as repository-level changes first,
then call out skill-specific notes if needed.

## Single-repo guardrails

Keeping everything in one repository is fine as long as the skills remain
portable.

Keep these boundaries stable:

1. Each skill lives under `skills/<skill-name>/`.
2. Each skill directory must contain `SKILL.md`.
3. Each `SKILL.md` keeps YAML frontmatter with at least:
   - `name`
   - `description`
4. Skill references should stay relative and resolve within the published
   `skills/` tree for the same repository snapshot.
5. Cross-skill references are allowed when they point to companion skills that
   ship under the same repository tag.
6. Do not make `SKILL.md` depend on unrelated repository docs or source files
   outside `skills/`.
7. Treat published skill names and directory names as stable install identifiers.

These rules keep the repository easy to use now and easy to split later if
that becomes necessary.

## Why the combined model is the default

This project currently prefers one repository because:

- there is only one place to edit and review skills
- the skills are companion artifacts for this specific MCP server
- the maintenance overhead stays low while the skill set is still small
- users can already install the skills from the same GitHub repository

## When to split later

Do not split preemptively. Consider a dedicated skills repository only if one
or more of these becomes true:

- the skills need a release cadence separate from the server
- the root README becomes too confusing for server users vs skill users
- the skill tree starts needing review, ownership, or issue triage independent of the server
- the skills need to be mirrored, versioned, or distributed independently
- the skills stop being companion artifacts and become a standalone product surface

## Future split path

If a split is needed later, the migration target is straightforward because the
skills already live under `skills/`:

1. Copy or mirror `skills/` into a dedicated repository.
2. Keep the same skill directory names and `name` frontmatter values.
3. Add a focused README with install commands.
4. Update this repository README to point to the new install source.

Because the current rules keep dependencies inside the published `skills/`
tree, the simplest split path is still a repository move. If only a subset of
skills moves later, update or remove any cross-skill references first.

## What not to change

Avoid these adjustments unless there is a separate product reason:

- do not add npm runtime dependencies just for skill distribution
- do not add MCP tools for installing skills
- do not couple skill install instructions to prompt catalog runtime behavior
- do not rename published skill directories casually; that breaks install references

## Manual verification checklist

- confirm each published skill directory contains `SKILL.md`
- confirm frontmatter `name` matches the intended install identifier
- confirm every relative reference in each published skill resolves inside the
  published `skills/` tree for the same repository snapshot
- confirm any cross-skill dependency points to a companion skill shipped under
  the same repository tag
- confirm README install examples point to this repository and include `--skill`
- confirm skill docs do not rely on unrelated top-level repository files
