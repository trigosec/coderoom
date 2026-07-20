# Package design: internal/shell

## Scope

`internal/shell` is the synchronous process runner for prompt-language shell
statements. It executes an explicit user-authored program, collects its result,
and has no dependency on the TUI, room, session, or prompt-language parser.

The caller owns concurrency and presentation. In particular, a future TUI
integration must call the runner outside the Bubble Tea update loop.

## Execution

The runner invokes:

```text
/bin/sh -c <program>
```

The process:

- runs in the caller-provided working directory
- inherits the Code Room process environment
- captures standard output and standard error separately
- runs in its own process group, which is terminated when the supplied context
  is cancelled so pipeline children do not outlive the shell
- bounds the wait for inherited output pipes after the root shell exits, then
  terminates any remaining background descendants in the process group

This targets the Linux and macOS platforms currently supported by Code Room.

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
