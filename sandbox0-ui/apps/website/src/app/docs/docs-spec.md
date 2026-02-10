# Sandbox0 Docs Spec (Current-State Based)

## Goal

Replace placeholder docs under `apps/website/src/app/docs` with documentation that matches current implementation and contracts.

This spec defines:

- top-level information architecture
- page-level taxonomy and scope
- source-of-truth mapping for each section
- writing rules for multi-language examples via Tabs
- execution order for implementation

## Scope

In scope:

- `/docs` content architecture and page plan
- module docs for Sandbox, Volume, Template
- Self-hosted docs focused on infra-operator usage
- Get Started onboarding docs

Out of scope (for this phase):

- design system / visual redesign
- new product features not present in current APIs/services
- per-SDK dedicated section trees (use Tabs instead)

## Authoritative Sources (Must Follow)

Always derive technical content from:

- `infra/pkg/apispec/openapi.yaml` (API contract)
- `infra/tests/e2e/cases/*` (verified behavior)
- `sdk-go/examples/*` (real usage patterns)
- `infra/infra-operator/api/v1alpha1/sandbox0infra_types.go` (self-hosted config model)
- `infra/infra-operator/chart/samples/**` (deploy scenarios)
- `infra/storage-proxy/pkg/juicefs/README.md` and `infra/k8s-plugin/README.md` (operator-related troubleshooting only)
- workspace rule in `CLAUDE.md` (current architecture and constraints)

Do not use existing placeholder docs as source of truth.

## IA (Top-Level Directories)

Top-level docs sections:

1. Get Started
2. Sandbox
3. Volume
4. Template
5. Self-hosted

## Detailed Taxonomy

### 1) Get Started

Purpose: first successful workflow for new users, based on current APIs.

Subsections:

- Overview
  - what Sandbox0 is
  - control plane vs data plane (high level)
- Quickstart
  - install client dependencies
  - authenticate
  - claim first sandbox
  - run first command/context
  - basic file operation
- Authentication
  - login/register model
  - API key usage
  - token notes and team context
- Core Concepts
  - sandbox lifecycle states
  - context types (REPL vs Cmd)
  - template and volume relationships
  - network policy basics
- First End-to-End Example (Tabs)
  - Go / Python / JavaScript / CLI in one page via `<Tabs>`

### 2) Sandbox

Purpose: lifecycle, execution, files, and network controls for sandboxes.

Subsections:

- Overview
  - sandbox model and key IDs
- Create & Manage
  - claim sandbox
  - get details
  - patch settings
  - delete sandbox
  - pause/resume/refresh
- Contexts (Process Runtime)
  - create/list/get/delete context
  - restart/input/resize/signal
  - run command (`exec`) patterns
  - WebSocket stream usage
- Files
  - read/write/delete
  - stat/list/move/mkdir
  - watch files via websocket
- Network Policy
  - get/patch network policy
  - mode and egress rules
- API Reference Mapping
  - endpoint index with links to OpenAPI-backed behavior

### 3) Volume

Purpose: persistent storage lifecycle and attachment workflow.

Subsections:

- Overview
  - why volumes exist
  - relation to sandbox lifecycle
- Volume Lifecycle
  - create/list/get/delete volume
  - metadata and ownership
- Mount & Unmount
  - mount request fields
  - mount status
  - unmount semantics
- Snapshots
  - create/list/get/delete snapshot
  - restore snapshot to volume
- Access Modes & Performance Options
  - RWO/ROX/RWX usage guidance
  - cache/prefetch/buffer/writeback parameters
- Troubleshooting
  - common mount/snapshot failures and checks

### 4) Template

Purpose: define and manage reusable runtime blueprints.

Subsections:

- Overview
  - what a template controls
  - template vs sandbox instance
- Template CRUD
  - list/create/get/update/delete template
- SandboxTemplateSpec Deep Dive
  - mainContainer
  - sidecars
  - pool (`minIdle`, `maxIdle`, `autoScale`)
  - lifecycle
  - network
  - env vars
  - pod overrides
- Visibility & Governance
  - public/private scope
  - allowed teams
- Registry Credentials
  - configure image registry auth
- Best Practices
  - stable image pinning
  - startup performance tips

### 5) Self-hosted

Purpose: provide a simple, operator-first self-hosted guide where users mainly interact with `infra-operator`.

Subsections:

- Overview
  - what infra-operator manages for users
  - operator-first workflow and boundaries
- Architecture
  - control plane and data plane overview
  - component responsibilities (high level only)
- Prerequisites
  - Kubernetes baseline requirements
  - minimal external dependencies checklist
- Install Infra Operator
  - helm install flow
  - initial verification checklist
- Deploy Scenarios
  - minimal mode
  - full mode
  - volumes mode
  - network-policy mode
- Configuration Reference
  - common `Sandbox0Infra` fields users must set
  - database/storage/registry quick mapping
  - init user bootstrap
- Upgrade & Day-2
  - upgrade operator/chart versions
  - safe rollout/rollback checklist
- Troubleshooting
  - install failures
  - reconciliation and runtime diagnostics
  - storage/FUSE related quick checks

## Writing Rules (Docs Implementation Standards)

- Keep docs aligned with current code and OpenAPI; avoid speculative content.
- Use Markdown as default, MDX components only when needed.
- Use `<Tabs>` for multi-language snippets; do not create separate subtrees per SDK language.
- Tabs standard labels: `Go`, `Python`, `JavaScript`, `CLI`.
- Keep API endpoints in `/api/v1/...` format when sourced from OpenAPI.
- Every behavior claim should be traceable to one authoritative source path.

## Tabs Strategy (Cross-Section)

Use the same language ordering across all pages:

1. Go
2. Python
3. JavaScript
4. CLI

Guidelines:

- If a language example is not yet implemented/verified, mark it clearly as planned and keep runnable examples only for validated languages.
- Reuse one canonical flow per topic and map language snippets to identical semantics.
- Prefer short, composable snippets over large end-to-end scripts.

## Page Inventory Plan (Target State)

Planned route groups under `src/app/docs`:

- `page.mdx` (docs home)
- `get-started/page.mdx`
- `get-started/authentication/page.mdx`
- `get-started/concepts/page.mdx`
- `sandbox/page.mdx`
- `sandbox/contexts/page.mdx`
- `sandbox/files/page.mdx`
- `sandbox/network/page.mdx`
- `volume/page.mdx`
- `volume/mounts/page.mdx`
- `volume/snapshots/page.mdx`
- `template/page.mdx`
- `template/spec/page.mdx`
- `template/visibility/page.mdx`
- `self-hosted/page.mdx`
- `self-hosted/architecture/page.mdx`
- `self-hosted/install/page.mdx`
- `self-hosted/deploy-scenarios/page.mdx`
- `self-hosted/configuration/page.mdx`
- `self-hosted/upgrade/page.mdx`
- `self-hosted/troubleshooting/page.mdx`

Note: final route split can be adjusted during implementation, but the five top-level modules are fixed.

## Execution Plan (Implementation Phases)

### Phase 1 - Baseline Reset

- remove/replace placeholder docs content under existing `/docs` pages
- create new docs home aligned with 5 top-level modules

### Phase 2 - Get Started + Sandbox

- deliver onboarding and core sandbox operation docs
- validate commands/examples against current API behavior

### Phase 3 - Volume + Template

- add persistent storage and template authoring docs
- include snapshot and pool-related best practices

### Phase 4 - Self-hosted

- add operator-first docs: architecture, install, deploy scenarios, configuration
- add focused day-2 and troubleshooting pages tied to infra-operator usage

### Phase 5 - Consistency Pass

- unify terminology and tabs style across all pages
- check internal links and route coverage
- ensure no placeholder wording remains

## Acceptance Criteria

- Information architecture contains exactly the five required top-level categories.
- Each category has complete, implementation-ready subcategories.
- No page depends on placeholder docs as source of truth.
- Multi-language docs are implemented with Tabs, not per-language section trees.
- Content is traceable to current repo state and contracts.
