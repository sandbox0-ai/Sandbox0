---
name: sandbox0
description: Use this skill when an AI agent developer wants to add Sandbox0 sandboxing to their own agent through the CLI or SDK, or needs help choosing templates, contexts, volumes, network policy, ports, webhooks, or self-hosted deployment. It uses only files bundled inside the skill.
---

# Sandbox0

Use this skill for Sandbox0 product usage and integration guidance for AI agent developers.

## Start here

1. Identify the user's integration goal.
2. Read `references/integration-map.md`.
3. Read only the relevant page under `references/docs-src/`.

## Typical triggers

- Add Sandbox0 to an AI agent project
- Pick between REPL and one-shot execution
- Persist workspace state across runs
- Design a template or custom image
- Restrict network access
- Expose a preview server or local web app
- Receive sandbox events through webhooks
- Decide whether self-hosted Sandbox0 is needed

## Working rules

- Prefer s0 CLI- and SDK-oriented guidance over internal implementation details.
- Recommend concrete compositions when helpful, for example `REPL + Volume` for persistent coding sessions.
- Use architecture detail only when it changes the user's design choice, security model, or deployment shape.

## Which references to read

- Topic routing and common solution shapes:
  `references/integration-map.md`
- Product docs and examples:
  `references/docs-src/<section>/.../page.mdx`
- Table of contents:
  `references/docs-src/manifest.json`
