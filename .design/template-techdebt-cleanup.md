# Template Common Files: Tech Debt Cleanup

## Problem

The codebase has two copies of shared agent home files (`.tmux.conf`, `.zshrc`, `.gitconfig`, `.geminiignore`):

| Location | Purpose | Status |
|----------|---------|--------|
| `pkg/config/embeds/common/` | Legacy seeding via `SeedCommonFiles()` / `SeedCommonFilesToHome()` | **Dead code** |
| `pkg/config/embeds/templates/default/home/` | Template inheritance via `SeedAgnosticTemplate()` | **Active** |

The files are byte-for-byte identical. Any change must be manually applied to both, and forgetting one creates silent drift (as happened with the `extended-keys` tmux fix).

## Root Cause

The original design (`.design/agnostic-template-design.md` section 4.7) planned for common files to live in `embeds/common/` and be seeded separately. However, commit `0c2a954` ("feat: template inheritance - default template as base layer") chose a different approach: embed the common files directly into the default template so `SeedAgnosticTemplate()` copies everything in one pass via `fs.WalkDir()`.

After that commit, no code was updated to remove the old `common/` directory or its associated functions. The design doc was also not updated to reflect the implementation.

## Current State

### Dead Code

| Symbol | Location | Call Sites |
|--------|----------|-----------|
| `SeedCommonFiles()` | `pkg/config/init.go:83-145` | 0 (dead) |
| `SeedCommonFilesToHome()` | `pkg/config/init.go:150-192` | 0 (dead) |
| `embeds/common/` directory | `pkg/config/embeds/common/` | Only referenced by dead functions above |

### Active Code

| Symbol | Location | Call Sites |
|--------|----------|-----------|
| `SeedAgnosticTemplate()` | `pkg/config/init.go:289-344` | `InitMachine()`, `NewTemplate()`, `UpdateDefaultTemplates()`, `TemplateImportWriter.Write()`, tests |

### Agent Provisioning Flow

The actual path `.tmux.conf` takes to reach an agent:

```
embeds/templates/default/home/.tmux.conf
    -> SeedAgnosticTemplate() copies to ~/.fabric/templates/default/home/.tmux.conf
    -> During provisioning, GetTemplateChainInGrove() always includes "default" as base
    -> provision.go copies template chain home/ dirs into agent home (template wins on conflict)
    -> Agent container mounts agent home at /home/fabric/
```

No harness-specific embeds (`pkg/harness/*/embeds/`) contain `.tmux.conf` or other common files.

## Proposed Cleanup

### Step 1: Delete Dead Code

Remove the two unused functions and the `embeds/common/` directory:

```
DELETE  pkg/config/embeds/common/.tmux.conf
DELETE  pkg/config/embeds/common/.zshrc
DELETE  pkg/config/embeds/common/.gitconfig
DELETE  pkg/config/embeds/common/.geminiignore
DELETE  pkg/config/init.go: SeedCommonFiles()      (lines 83-145)
DELETE  pkg/config/init.go: SeedCommonFilesToHome() (lines 150-192)
```

Since the project is in alpha and these functions have zero callers, no deprecation period is needed.

### Step 2: Update Design Doc

Update `.design/agnostic-template-design.md` section 4.7 to reflect the actual implementation: common files live in `embeds/templates/default/home/` and are seeded as part of the default template, not separately.

### Step 3: Update Tests

Remove or update any test code that references `SeedCommonFiles` or `SeedCommonFilesToHome` (currently only in `harness_config_test.go`).

## Risk Assessment

**Low risk.** The functions being deleted have zero call sites. The `embeds/common/` directory is only referenced by those functions. All active code paths use `embeds/templates/default/home/` exclusively. Existing tests for `SeedAgnosticTemplate` already verify that home files (including `.tmux.conf`) are correctly copied.

## Out of Scope

- Refactoring how harness-specific embeds work (those are separate and not duplicated)
- Changing the template inheritance chain logic
- Modifying how `provision.go` composes agent home directories
