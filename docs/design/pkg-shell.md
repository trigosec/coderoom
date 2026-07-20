# Package design: internal/shell

## Scope

`internal/shell` is the synchronous process runner for prompt-language shell
statements. It executes an explicit user-authored program, collects its result,
and has no dependency on the TUI, room, session, or prompt-language parser.

The caller owns concurrency and presentation. The TUI integration calls the
runner through a Bubble Tea command so execution does not block the update
loop, then maps the completed result into a canonical room command record. The
CLI application context is passed into the UI, which derives a child context
used by shell executions. The UI tracks active local executions and shutdown
cancels that context, prevents new executions from starting, and waits until
the active runners have terminated and reaped their processes.

## Execution

The runner invokes:

```text
/bin/sh -c <program>
```

The process:

- runs in the caller-provided working directory
- inherits the coderoom process environment
- captures standard output and standard error separately
- runs in its own process group, which is terminated when the supplied context
  is cancelled so pipeline children do not outlive the shell
- bounds the wait for inherited output pipes after the root shell exits, then
  terminates any remaining background descendants in the process group

This targets the Linux and macOS platforms currently supported by coderoom.

## Result

Every invocation returns a result containing:

- `Status`: `success`, `failure`, or `cancelled`
- `ExitCode`: present when the program exits normally, including non-zero exits
- `Stdout` and `Stderr`: captured independently
- `Err`: set for process-start failures and context cancellation

A non-zero program exit is a normal completed process result, not a runner
error. Its status is `failure`, its exit code and output are retained, and
`Err` remains nil.

## Non-goals

This package does not:

- parse `/shell` syntax
- render command results
- provide user-facing cancellation commands
- sequence prompt-language statements
- apply output limits or persistence
