# Trace File Naming and Directory Structure

**Observed:** 2026-08-26

**Root:** `.beads/traces/`

This document records the naming and layout of the bead execution traces. The
trace tree is runtime-generated and is ignored by Git, so directory and file
counts will change as beads run. The earlier [file type survey](trace-directory-survey.md)
documents the sampled size ranges.

## Directory hierarchy

The hierarchy is flat below the trace root. One child directory represents the
trace for one bead/session; there are no date, provider, model, or status
subdirectories.

```text
.beads/
└── traces/
    └── spaxel-<8-hex-id>/
        ├── metadata.json
        ├── stderr.txt
        ├── stdout.txt
        └── trace.jsonl       # optional; present when structured events were emitted
```

At the observation above, 98 bead directories existed. Of those, 97 had the
standard `metadata.json`, `stderr.txt`, and `stdout.txt` files; 88 also had
`trace.jsonl`. One newly-created directory was empty, demonstrating that a
trace directory can exist before a run has flushed its artifacts.

## Naming conventions

| Path component | Observed pattern | Meaning |
| --- | --- | --- |
| Trace directory | `spaxel-[0-9a-f]{8}` | The `spaxel-` project prefix followed by the bead identifier’s eight-character lowercase hexadecimal key. |
| Metadata | `metadata.json` | Fixed name for session metadata. |
| Standard output | `stdout.txt` | Fixed name for captured standard output. |
| Standard error | `stderr.txt` | Fixed name for captured standard error. |
| Structured events | `trace.jsonl` | Fixed name for newline-delimited structured events; optional in the observed corpus. |

The directory key is not a timestamp and is not a full UUID. All 98 observed
directory names matched `spaxel-[0-9a-f]{8}`, and each of the 97 metadata files
had a `bead_id` equal to its parent directory name. There is no timestamp,
sequence number, provider, or model in any observed path or artifact basename.

Timestamps live in file contents instead:

- `metadata.json.captured_at` uses UTC ISO 8601/RFC 3339 form, with nine
  fractional-second digits in the observed records, for example
  `2026-08-24T09:01:13.485670830Z`.
- Each `trace.jsonl` event has a numeric `ts` containing Unix epoch seconds
  with a fractional part.
- `metadata.json.duration_ms` records elapsed duration as milliseconds.

Consequently, directory names must be treated as identifiers rather than
chronological sort keys. Use `captured_at` or event `ts` when ordering runs.

## File roles and structural variation

`metadata.json` contains the session identity and execution summary. The
observed schema includes `bead_id`, `agent`, `provider`, `model`, `exit_code`,
`outcome`, `duration_ms`, `input_tokens`, `output_tokens`, `cost_usd`,
`captured_at`, `trace_format`, `pruned`, `template_version`, and
`timeout_reason`.

When present, `trace.jsonl` contains one JSON object per line. Every inspected
event used `schema_version: 1` and included `ts` and `type`. Observed event
types were:

- `agent_message`, with `role` and `content`;
- `tool_call`, with `tool` and `args` (and sometimes `path`);
- `tool_result`, with `tool`, `success`, and `output`; and
- `error`, with `message`, `code`, and `recoverable`.

The fixed text files are the human-readable process streams. They remain
present even when the structured JSONL file is absent; absence of
`trace.jsonl` is an artifact variation, not a separate directory category.

## Grouping and categorization

The filesystem grouping key is only the bead ID: `.beads/traces/<bead-id>/`.
Categorization by execution details is metadata-driven rather than encoded in
directories or filenames:

- `provider`, `agent`, and `model` identify the trace producer;
- `trace_format` identifies the serialization family (the snapshot contained
  `claude_json` and `openai_jsonl`); and
- `outcome`, `exit_code`, `pruned`, and `timeout_reason` describe completion
  state or retention state.

For tooling, enumerate the immediate child directories, validate the
`spaxel-[0-9a-f]{8}` key, then read `metadata.json` before interpreting the
optional files. Do not infer date, provider, or completion status from a path
name alone, and tolerate a directory that is still being written.
