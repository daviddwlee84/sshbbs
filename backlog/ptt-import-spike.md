# Article import from real PTT `.DIR` dumps

**Status**: P? — needs spike before committing
**Effort**: ?/L (unknown size, almost certainly large)
**Related**: `TODO.md`, `docs/ptt_trace_code/03_fileheader_dir.md`, `internal/store/articles.go`

## Context

2026-04, raised when scoping P0 — explicitly deferred from the original
plan as "P2-6". Real-PTT exports would let us bootstrap demo databases
that look like a lived-in board, not three seeded examples. Useful for
load testing too (a busy board has tens of thousands of articles).

The format is well-documented (see `docs/ptt_trace_code/03_fileheader_dir.md`):
`.DIR` is a packed array of fixed-width `fileheader_t` records, each
pointing to a per-article body file in the same `boards/<brdname>/`
directory. The challenge isn't the format — it's that pttbbs uses Big5
internally and we committed to UTF-8.

## Investigation

Not started. The spike below would be the first concrete work.

Things known up-front:

- `Ptt-official-app/go-bbs` — Go library that reads/writes pttbbs file
  formats. Quality and Big5-handling fidelity unknown.
- `golang.org/x/text/encoding/traditionalchinese.Big5` — standard library
  for Big5 transcoding. Edge cases: PTT historically used a Big5 superset
  with custom code points for emoji-like glyphs that don't roundtrip
  cleanly to Unicode.
- Article addressing: pttbbs uses recno (position in `.DIR`); we use
  auto-increment `id`. Import has to assign new IDs and either drop
  cross-references or build a recno→id map and rewrite them.

## Options considered

| Option | Pros | Cons |
|---|---|---|
| A. Use `go-bbs` library directly | Tested against real PTT data; handles edge cases | Library quality unknown; pulls in transitive deps; might not expose Big5 transcoding cleanly |
| B. Hand-write a `.DIR` parser using `encoding/binary` | Full control; minimal deps | More code to maintain; we'd re-discover Big5 quirks the hard way |
| C. Skip imports, lean on the Seed-data CLI (P2 active) for synthetic content | No dependency, no maintenance | Won't help users who want to mirror an existing board |

Likely answer: **A**, but only after the spike. **C** is already on the
P2 lane and is independently useful regardless.

## Spike checklist (defines the "?" in `?/L`)

Before promoting from P? to P1/P2, the spike must answer:

- [ ] Build a tiny CLI that reads one published PTT `.DIR` + 5 article
      bodies via `go-bbs`. Does it work? How invasive is the integration?
- [ ] Round-trip 100 articles' titles + bodies through Big5 → UTF-8 →
      Big5. Identify the lossy code points. Is the loss acceptable for
      a read-only mirror?
- [ ] Estimate import throughput for ~10k articles. Will the existing
      `writeMu` serialize so badly that bulk import takes hours, or do
      we need a bulk-load path that bypasses it?
- [ ] Decide what to do with pushes — pttbbs stores them either as
      counters or in a separate `commentd` daemon's storage. If the
      latter, do exports include them?

## Current blocker / open questions

- Need a sample `.DIR` + bodies dump from a real (or test) PTT
  installation. Public PTT data isn't a thing; we'd need to either ask
  someone running pttbbs locally or stand up a pttbbs instance ourselves.
- Open: do we want imports to be append-only (per board, with a marker
  for "imported from pttbbs at $ts") or a wholesale board-level wipe
  + re-import? Affects schema design.

## Decision (if any)

None yet. Re-evaluate when someone has a concrete reason to import (e.g.
"I want to seed the demo with the gossip board's hot posts").

## References

- `docs/ptt_trace_code/03_fileheader_dir.md` — our internal mapping doc
- `Ptt-official-app/go-bbs` — the candidate library
- `golang.org/x/text/encoding/traditionalchinese`
