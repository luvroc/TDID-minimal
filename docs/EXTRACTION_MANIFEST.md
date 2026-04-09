# Extraction Manifest

## Source

- extraction date: `2026-04-09`
- tee source repo: internal TEE-side prototype repository
- chain source repo: internal chain-side prototype repository

## Selection rule

This public subset keeps:

- the current prototype source path on the TEE side
- the current contracts / chaincodes and related deployment scripts on the chain side
- the `T0` serial baseline workflow and its direct helper scripts
- `context-sharing` materials for later follow-up development

This public subset excludes:

- large deployment artifacts and packaged bundles
- local git state
- built binaries and generated Occlum instance outputs
- agent reports and unrelated experiment leftovers

## Current top-level layout

- `tee/`
- `chain/`
- `scripts/`
- `docs/`

## Practical note

This is intended as a clean starting point for open-sourcing the TDID paper prototype, not as a fully sanitized release candidate. A final credential / endpoint review is still recommended before publication.
