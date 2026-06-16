# OpenCode MiMo Feature Port Design

Date: 2026-06-15

## Goal

Build a personal custom OpenCode fork that keeps official OpenCode as the primary upstream while selectively porting the most useful MiMo-Code features.

Official upstream:

- `https://github.com/anomalyco/opencode`
- Default branch: `dev`

Feature reference:

- `https://github.com/XiaomiMiMo/MiMo-Code`
- Default branch: `main`

The result should feel like OpenCode with selected MiMo-style productivity features, not like a wholesale MiMo rebrand.

## Non-Goals

- Do not merge MiMo-Code wholesale.
- Do not adopt Xiaomi-specific provider policy, telemetry, branding, or hosted-service assumptions unless explicitly desired later.
- Do not replace OpenCode's config format unless a feature requires a small, validated schema addition.
- Do not implement every MiMo feature in the first pass.

## Selected Features

Port these features in phases, from lowest risk to highest risk.

## Phase 0: Repository Setup

Create a dedicated source checkout, for example `~/CODE/opencode-custom`.

Remotes:

- `origin`: the user's fork of official OpenCode
- `upstream`: `https://github.com/anomalyco/opencode.git`
- `mimo`: `https://github.com/XiaomiMiMo/MiMo-Code.git`

Base branch:

- Track `upstream/dev`.
- Use feature branches such as `custom/tps`, `custom/ghost-prompt`, `custom/compose`, and `custom/memory`.

Each ported feature should land as an isolated commit or short commit series so future rebases against official OpenCode remain understandable.

## Phase 1: Low-Risk UI Features

### Token Per Second

Purpose:

Show model response speed in the TUI, similar to MiMo-Code's sidebar readout.

MiMo reference files:

- `packages/opencode/src/cli/cmd/tui/feature-plugins/sidebar/tps.ts`
- `packages/opencode/src/cli/cmd/tui/feature-plugins/sidebar/context.tsx`

Design:

- Add a small TPS helper that estimates streaming speed while an assistant message is still generating.
- Use completed assistant message token counts after generation finishes.
- Prefer an existing OpenCode sidebar/status slot if present. If OpenCode lacks the MiMo sidebar plugin structure, add the smallest TUI integration point available.
- Display compact text such as `42 t/s`.

Risks:

- OpenCode's current TUI plugin API may differ from MiMo's fork.
- Streaming token estimates are approximate because partial text is estimated before final provider usage arrives.

Validation:

- Typecheck TUI package.
- Start custom OpenCode and confirm TPS appears during and after model output.

### Ghost Prompt Suggestion

Purpose:

After the assistant finishes a turn, show a suggested next prompt as ghost text in an empty input. The user can accept it with Tab.

MiMo reference file:

- `packages/opencode/src/cli/cmd/tui/component/prompt/index.tsx`

Design:

- Add client-side ghost text state to the prompt input component.
- Trigger prediction only when a session transitions to idle and the input is empty.
- Invalidate predictions if the session changes, a new run starts, the conversation advances, or the user types.
- Accept suggestion with Tab only when autocomplete is not open.
- Add or port a small `session.predict` API path if official OpenCode does not already expose one.

Risks:

- Requires both UI changes and backend prediction support.
- Must not interfere with existing autocomplete, slash commands, file mentions, or Tab agent switching.

Validation:

- Confirm normal typing, slash command autocomplete, and file mention autocomplete still work.
- Confirm ghost text appears only after idle transition and only when input is empty.
- Confirm Tab accepts ghost text without breaking command keybinds.

## Phase 2: Workflow Features

### Compose Mode

Purpose:

Add a structured workflow mode for larger tasks: understand requirements, write a spec, create an implementation plan, execute with verification, and review before completion.

MiMo reference:

- README feature description for `compose`
- Agent definitions and prompts in the MiMo source tree
- Existing OpenCode agent and command schema

Design:

- Start with a primary agent named `compose` rather than a large runtime rewrite.
- The `compose` agent should orchestrate a spec-driven workflow and use existing OpenCode skills/agents where possible.
- Keep `build` and `plan` unchanged.
- Add config defaults or bundled agent definitions only if official OpenCode's agent loader supports them cleanly.
- Use explicit checkpoints in the workflow text initially; deeper automatic checkpointing belongs to Phase 3.

Risks:

- A true MiMo-like compose mode may depend on memory/checkpoint internals.
- Over-porting compose too early could couple the fork to MiMo's runtime structure.

Validation:

- `Tab` can switch to compose if exposed as a primary agent.
- Compose can guide a task from spec to verification without requiring memory core.
- Existing `build`, `plan`, `general`, and `explore` agents continue to work.

### Goal Stop Condition

Purpose:

Let the user define a goal for the session and prevent premature completion when the work is not actually done.

MiMo reference:

- `packages/opencode/src/session/goal.ts`

Design:

- Add a command or session state field for goal text.
- Before the assistant stops after autonomous work, run a lightweight judge check only when a goal is active.
- If the judge says the goal is not satisfied, continue with a concrete next action or ask the user if blocked.
- Keep this feature optional and easy to disable.

Risks:

- Can increase token usage.
- A bad judge prompt can create loops.

Validation:

- Goal can be set, inspected, and cleared.
- The assistant can stop normally when no goal is set.
- The assistant does not loop indefinitely when blocked.

## Phase 3: Memory Core

### Persistent Memory And Checkpoints

Purpose:

Preserve project knowledge, session progress, task state, and important decisions across sessions.

MiMo reference files:

- `packages/opencode/src/memory/*`
- `packages/opencode/src/session/checkpoint.ts`
- `packages/opencode/src/session/checkpoint-paths.ts`
- `packages/opencode/src/session/checkpoint-templates.ts`
- `packages/opencode/src/session/checkpoint-context.ts`
- `packages/opencode/src/session/prune.ts`
- `packages/opencode/src/session/compaction.ts`
- `packages/opencode/src/project/bootstrap.ts`

Design:

- Add memory storage under OpenCode's normal data/state directories, not under MiMo-specific paths.
- Keep file names familiar and explicit: `MEMORY.md`, `checkpoint.md`, `notes.md`, and task progress files.
- Add a memory service boundary that owns paths, reconciliation, indexing, and search.
- Inject memory into session context through a narrow prefix/context assembly layer.
- Add checkpoint writer as an internal subagent or background task only after the basic memory service works.
- Add config flags such as `memory.auto`, `checkpoint.auto`, and budget limits only if validated against OpenCode's schema system.

Risks:

- This is the highest-risk port because it touches session lifecycle, storage, context construction, and compaction.
- Schema or database migrations may diverge between OpenCode and MiMo.
- Bad memory injection can pollute prompts or leak stale information.

Validation:

- Existing sessions still load.
- New sessions create or read memory files without crashing.
- Context injection is visible in debug output.
- Checkpoint writer updates checkpoint files without corrupting session history.
- Compaction still preserves enough recent context.

## Phase 4: Self-Improvement Features

### Dream And Distill

Purpose:

Consolidate durable knowledge and discover repeated workflows that should become reusable skills, agents, or commands.

MiMo reference:

- `packages/opencode/src/session/auto-dream.ts`
- MiMo `/dream` and `/distill` command behavior

Design:

- Implement manual commands first: `/dream` and `/distill`.
- Auto-run later behind config flags and time intervals.
- Dream should update project memory only with durable, verified knowledge.
- Distill should propose workflow candidates first, then create assets only when confidence is high or the user approves.

Risks:

- Auto-distill can create noisy or duplicate skills.
- Dream can persist incorrect assumptions if not conservative.

Validation:

- Manual dream updates memory with concise project facts.
- Manual distill lists repeated workflows before creating files.
- Auto-run can be disabled.

## Data Flow

For normal chat:

1. User enters prompt in TUI.
2. Session context builder loads agent prompt, instructions, config, and later memory/checkpoint context.
3. Provider streams assistant output.
4. TUI renders streaming output and TPS.
5. On completion, usage is stored.
6. Prompt input requests a prediction when idle and empty.
7. Optional checkpoint writer records durable progress.

For compose workflow:

1. User switches to compose or invokes compose command.
2. Compose asks clarifying questions only when needed.
3. Compose writes or updates a spec.
4. Compose creates an implementation plan.
5. Build/execution agent performs changes.
6. Verification runs before completion.
7. Goal judge or compose review prevents premature stop.

## Error Handling

- If MiMo donor code depends on Xiaomi-specific modules, isolate or remove that dependency.
- If a port requires schema changes, validate with the generated/config schema before using it.
- If prediction fails, silently omit ghost prompt suggestions.
- If TPS cannot be computed, hide the TPS line rather than showing `NaN` or `0 t/s`.
- If memory indexing fails, warn but keep OpenCode usable.
- If checkpoint writing fails, keep the main session alive and surface a non-blocking warning.

## Testing Strategy

Use progressive verification after each phase.

Per feature:

- Run typecheck.
- Run relevant unit tests if available.
- Build the CLI binary.
- Launch TUI and manually smoke-test the changed behavior.

Regression checks:

- Existing providers still load.
- Existing agents still switch correctly.
- Slash commands and autocomplete still work.
- MCP config still loads.
- Existing sessions still open.

## Maintenance Strategy

- Keep official OpenCode as the primary upstream.
- Regularly rebase or merge from `upstream/dev`.
- Keep MiMo only as a donor remote.
- Document every ported feature with source file references.
- Avoid copying provider restrictions, telemetry, or hosted-service assumptions.
- Prefer small, reviewable commits over large merges.

## Initial Implementation Order

1. Create or clone the official OpenCode fork.
2. Add `upstream` and `mimo` remotes.
3. Confirm official OpenCode builds before custom changes.
4. Port TPS.
5. Port ghost prompt suggestion.
6. Add compose as primary agent/workflow.
7. Add goal stop condition.
8. Port memory service.
9. Port checkpoint writer and context injection.
10. Add dream/distill manual commands.
11. Add auto dream/distill only after manual commands are stable.

## Initial Decisions

- Binary alias: use `opencode-custom` during development so stable `opencode` remains available.
- Config path: keep a separate development config if feasible. If the official OpenCode config loader makes this too invasive, share `~/.config/opencode` but avoid destructive config migrations.
- Ghost prediction model: use configured `small_model` when present, otherwise use the current session model.
- Goal judge model: use configured `small_model` when present to reduce cost.
- Compose availability: expose `compose` as a selectable primary agent, but do not make it the default agent until it is stable.
- Feature flags: default complex features to opt-in during development, especially goal judging, memory/checkpoint, dream, and distill.
