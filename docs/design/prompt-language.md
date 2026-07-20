# coderoom Prompt Language: Version 0

## Status

This document proposes the first executable slice of a prompt-based language
for coderoom. It incorporates the existing room commands and adds the minimum
syntax needed to define a reusable shell-backed command and use it as the
termination condition for a bounded agent loop.

The canonical Version 0 program is:

```text
/def tests /shell go test ./...
/loop @ada make the tests pass without weakening them /until /tests /max 3
```

The purpose of this slice is to validate the language model, not to anticipate
the complete language. In particular, Version 0 does not add content variables,
parameters, general expressions, conditionals, or multi-command function
bodies.

## Design Goals

- Preserve the existing interactive room syntax.
- Keep agent execution sequential, as it is today.
- Let users extend coderoom without adding workflow-specific built-ins.
- Treat deterministic external commands as reusable success conditions.
- Make agent loops bounded and visible to the human.
- Leave room for future content variables and richer command definitions.

## Existing Language

coderoom already recognizes three forms of input:

```text
@ada inspect the current implementation   # direct shared-room send
Please inspect the current implementation # broadcast
/handoff ada turing                       # built-in command
```

The current user-facing built-ins are:

```text
/invite <alias>
/remove <alias>
/cancel <alias>
/handoff <from> <to>
/who
/help
/quit
```

Version 0 preserves these forms. It adds `/shell`, `/def`, user-defined command
invocation, and `/loop`.

## Syntax

The grammar below uses simplified EBNF. `/shell` consumes the raw remainder of
its input, while `/loop` identifies its control clauses from the end of the
line and treats the text between its participant and those clauses as a prompt.

```ebnf
input               = statement ;

statement           = participant_send
                    | broadcast
                    | builtin_command
                    | user_command ;

participant_send    = participant_reference, free_text ;
broadcast           = free_text ;

builtin_command     = invite
                    | remove
                    | cancel
                    | handoff
                    | who
                    | help
                    | quit
                    | shell
                    | define
                    | loop ;

invite              = "/invite", identifier ;
remove              = "/remove", identifier ;
cancel              = "/cancel", identifier ;
handoff             = "/handoff", identifier, identifier ;
who                 = "/who" ;
help                = "/help" ;
quit                = "/quit" ;

shell               = "/shell", shell_program ;
define              = "/def", identifier, command_expression ;
loop                = "/loop", participant_reference, loop_prompt,
                      "/until", command_reference, "/max", integer ;

command_expression  = shell ;
user_command        = command_reference ;
command_reference   = "/", identifier ;
participant_reference = "@", identifier ;

identifier          = identifier_start, { identifier_part } ;
identifier_start    = letter ;
identifier_part     = letter | digit | "-" | "_" ;
loop_prompt         = ? non-empty text before the terminal /until clause ? ;
shell_program       = ? remaining non-empty input ? ;
free_text           = ? remaining non-empty input ? ;
integer             = digit, { digit } ;
```

`command_expression` intentionally contains only `/shell` in Version 0. This
makes the following a complete command definition rather than an eager command
execution:

```text
/def tests /shell go test ./...
```

The stored command is invoked by name:

```text
/tests
```

`/shell` consumes the remaining non-empty input as its program. coderoom trims
the whitespace immediately after `/shell`, then passes the rest to the shell as
written. Quotes inside that program retain their normal shell meaning:

```text
/shell echo "hello world"
/shell go test ./... | tee test-output.txt
```

The shell expression must therefore be the final part of a single-line command
definition in Version 0.

## Names and References

Participants and commands occupy separate namespaces:

```text
@tests # participant named tests
/tests # user-defined command named tests
```

Built-in command names are reserved. A definition such as `/def help ...` is
invalid because `/help` already exists. Redefining an existing user-defined
command is also an error in Version 0; an explicit replacement operation can be
designed later if needed.

Command names are written without a slash in a definition and with a slash at
the call site:

```text
/def tests /shell go test ./...
/tests
```

The `:` prefix remains available for future content variables. Version 0 does
not assign semantics to it.

## Command Results

Every executable command produces an internal result:

```text
CommandResult {
    status:    success | failure | cancelled
    exit_code: optional integer
    output:    text
    error:     text
}
```

`output` contains standard output. `error` contains standard error or an error
that prevented the command from starting. Keeping them separate preserves the
information even if the first UI representation chooses to collapse them into
one record.

The result is runtime state, not a user-visible value type in Version 0. It is
rendered in the room and consumed by `/loop`, but cannot yet be assigned to a
variable or inspected through expressions.

Existing commands map successful completion, validation failure, and
cancellation onto the same result contract where needed by the interpreter.

## `/shell`

`/shell` executes the remainder of its input in the coderoom workspace:

```text
/shell go test ./...
```

It collects the process exit code, standard output, and standard error.

- Exit code zero produces `success`.
- A non-zero exit code produces `failure`.
- Failure to start the process produces `failure` without an exit code.
- User cancellation produces `cancelled`.

The shell invocation, status, and output are visible in the room. `/shell` is
an explicit user action; an agent cannot cause coderoom to execute an
arbitrary host command merely by emitting this syntax in its response.

The concrete shell, environment inheritance, output limits, and cancellation
mechanism are implementation decisions that must be settled before `/shell` is
considered production-ready.

## `/def`

`/def` associates a name with an unevaluated command expression:

```text
/def tests /shell go test ./...
```

Defining `tests` does not execute the shell program. Invoking `/tests` evaluates
the stored expression and returns its result. A Version 0 definition has no
parameters, contains exactly one command expression, and returns that command's
result unchanged.

Definitions are scoped to the running room in Version 0. Persistence and
project-level command files are future design questions.

## `/loop`

`/loop` repeatedly asks one participant to change the workspace until a
user-defined command succeeds:

```text
/loop @ada make the tests pass without weakening them /until /tests /max 3
```

The parser resolves the statement from both ends. It reads `/loop` and the
participant reference from the start, then reads `/max <integer>` and
`/until <command-reference>` backwards from the end. The remaining non-empty
text is the participant prompt. This makes the embedded agent instruction look
like the ordinary `@alias <text>` form while keeping the loop controls
unambiguous.

Execution proceeds as follows:

1. Send the participant the loop prompt.
2. Wait for the participant's turn to finish using coderoom's existing command
   sequencing.
3. Invoke the command named after `/until`.
4. If it succeeds, finish the loop successfully.
5. If it is cancelled, cancel the loop.
6. If it fails and the maximum number of participant turns has been reached,
   finish the loop with the last failed result.
7. Send the participant the loop prompt together with the failed condition's
   name, exit code, output, and error.
8. Repeat from step 2.

`/max` counts participant turns, not condition evaluations. It is mandatory in
Version 0 so an agent loop cannot consume unbounded time or credits.

The assessment and every participant turn remain visible in the shared room.
The human can cancel the active participant with the existing `/cancel`
command; cancelling that turn cancels the loop rather than silently continuing.

The participant receives the original prompt on the first turn. After a failed
condition evaluation, the next turn receives a structured prompt equivalent to:

```text
Make the tests pass without weakening them

The completion condition is failing. Continue working on the task using
the evidence below.

Condition command: /tests
Status: failure
Exit code: 1
Stdout:
<standard output>

Stderr:
<standard error>

Error:
<execution error or (none)>
```

Empty result fields are shown as `(none)`. Version 0 passes the complete result
without truncation; if output limits are introduced later, the truncation must
be explicit in both the participant message and room record. None of the
collected evidence is silently discarded.

## Examples

Run a shell command directly:

```text
/shell go test ./...
```

Define and invoke it:

```text
/def tests /shell go test ./...
/tests
```

Use it as a bounded success condition:

```text
/def tests /shell go test ./...
/loop @ada fix the failing tests without changing their assertions /until /tests /max 3
```

Define another project-specific condition without adding a built-in:

```text
/def lint /shell go vet ./...
/loop @ada resolve the reported static-analysis findings /until /lint /max 2
```

## Invalid Version 0 Programs

```text
/def help /shell go test ./...     # conflicts with a built-in
/def tests                         # missing command expression
/def tests /who                    # unsupported definition body
/loop ada fix tests /until /tests /max 3      # participant requires @
/loop @ada /until /tests /max 3               # missing prompt
/loop @ada fix tests /until /tests             # missing bound
```

## Non-goals

Version 0 does not define:

- content variables or `/set`
- function parameters or return expressions
- multi-command function bodies
- `/if`, `/while`, `/break`, or `/continue`
- nested or concurrent loops
- arbitrary command expressions after `/until`
- persistent or repository-defined functions
- participant outputs as assignable values
- automatic commits, reverts, or acceptance decisions

These features should be designed from experience with the Version 0 execution
model rather than inferred in advance.

## Open Questions

- Which shell should `/shell` use on each supported platform?
- How much output should be retained and shown for long-running commands?
- Should user-defined commands persist in room state or live in a project file?
- Should `/def` eventually accept a block, parameters, or both?
- Should `/loop` accept an inline command expression as well as a named command?
- How should a failed participant turn differ from a cancelled turn at the loop
  boundary?
- Should future participant arguments consistently require `@`, including a
  future revision of `/handoff`?
