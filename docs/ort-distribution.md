# ORT (ONNX Runtime) Distribution Strategy

FractalBot’s Phase 3 semantic memory uses ONNX Runtime (ORT). ORT is a native library, so we need a safe, reproducible way to ship platform-specific binaries while keeping a **default-deny** posture.

This document describes supported options and the current recommendation.

## Goals

- **Safe by default**: no surprise downloads, no arbitrary paths, no secret leakage in logs/errors.
- **Reproducible**: pinned versions and integrity verification (checksums) where downloads are involved.
- **Works offline** when possible, especially for local demo/dev workflows.
- **CI-friendly**: deterministic tests; avoid network access in tests.

## Option A: Embed ORT binaries in-repo (current)

**What it is**

- Commit ORT binaries under `internal/memory/ort/lib/<os>/<arch>/...` and extract them at runtime into a cache directory.

**Pros**

- Offline-friendly: no network required to obtain ORT.
- Reproducible: the shipped bytes are exactly what runs (plus local extraction checksum verification).
- Simple operationally: no release plumbing.

**Cons**

- Repository size grows significantly (clone/fetch cost).
- Updating ORT requires committing new binaries.

**Security notes**

- Keep extraction directory fixed under a safe cache dir.
- Verify extracted bytes by checksum (already required).
- Ensure unsupported platforms fail clearly (no partial/undefined behavior).

## Option B: Move ORT binaries to GitHub Release assets (recommended future direction)

**What it is**

- Publish ORT binaries as release assets (per OS/arch).
- Provide a tool/installer step to download assets when explicitly requested.

**Pros**

- Keeps the git repository lean.
- Still reproducible if assets are pinned (tag/sha) and verified by checksum.

**Cons**

- Requires release process and asset hosting.
- Requires network when installing ORT (unless pre-fetched/cached).

**Security notes**

- Downloads must be **pinned** (release tag + asset name) and verified by **SHA256**.
- Download destination must be a safe cache directory (no user-controlled path traversal).
- Never log full URLs with query params; never print token-bearing URLs.

## Option C: Download ORT on demand at runtime (not recommended as default)

**What it is**

- If ORT is missing, the application downloads it automatically.

**Pros**

- “Just works” for users on supported platforms.
- Repo stays small.

**Cons**

- Network access at runtime is a larger trust surface and harder to reason about.
- Can surprise users and break default-deny expectations.

**Security notes**

- Must be explicitly **opt-in** (e.g., `agents.memory.allowDownloads: true`).
- Must be pinned + checksum-verified.
- Must use safe cache directory and strict error sanitization.
- Tests must not require network access (use `httptest`).

## Recommendation

**Keep Option A (embedded ORT) for now**, because it preserves offline, reproducible behavior and keeps runtime network default-deny. This is acceptable while Phase 3 memory is stabilizing and the supported platform set is small.

Plan to move to **Option B (release assets)** once:

- the memory feature is stable,
- we can commit to a pinned ORT version lifecycle,
- and we want to reduce repo/clone size.

If/when Option B is implemented, keep downloads **installer-only** (not runtime), pinned, and checksum-verified.

## CI expectations

- Docs-only changes should still keep CI green (`gofmt` + `go test ./...`).
- No new download or extraction behavior should be introduced by documentation-only PRs.

