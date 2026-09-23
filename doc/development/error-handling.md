# Error Handling

## Overview

Every error has two audiences:

* The **user** gets a short message, which describes what is wrong, together
  with a hint, which describes what to do about it. Implementation details are
  never reported to the user.
* The **log** gets the complete error chain including the technical details,
  exactly once per failure, correlated with the request through a request ID.

The handling is implemented in three places: `internal/domain` provides the
error type, `internal/util/response` turns an error into an HTTP response and
logs it, and `shared/api` defines what a client receives.

## What a client receives

An error response carries the message, the HTTP status code and metadata:

```json
{
  "type": "error",
  "error_code": 400,
  "error": "Server \"server01\" is a member of cluster \"one\" and can not be deleted",
  "metadata": {
    "reason": "server_is_cluster_member",
    "hint": "Remove the server from the cluster first.",
    "details": {"server": "server01", "cluster": "one"},
    "request_id": "3f1c1a4e-0a0e-4a5f-9a1e-2d7b8c9e0f11"
  }
}
```

The same request ID is reported in the `X-Request-Id` response header and with
every log record of the request.

Clients report the message and the hint, they do not invent guidance of their
own:

```
$ operations-center provisioning server remove server01
Error: Server "server01" is a member of cluster "one" and can not be deleted
Hint: Remove the server from the cluster first.
```

## Errors caused by the user

Report them with `domain.NewErrorf`, so the message reaches the user unaltered:

```go
return domain.NewErrorf(domain.ErrOperationNotPermitted, api.ErrorReasonServerIsClusterMember,
    "Server %q is a member of cluster %q and can not be deleted", name, cluster).
    WithHintf("Remove the server from the cluster first.").
    WithDetail("server", name).
    WithDetail("cluster", cluster)
```

* **Kind**: classifies the error and decides the status code. It is matched with
  `errors.Is`, so `errors.Is(err, domain.ErrOperationNotPermitted)` keeps working
  through wrapping.
* **Message**: what is wrong. It is self-contained, so it does not rely on the
  context added by other layers, names are quoted with `%q`, it is capitalized
  and carries no trailing period.
* **Reason**: optional, see [Reasons](#reasons).
* **Hint**: optional, see [Hints](#hints).
* **Details**: the dynamic values of the message in machine readable form.
* **Cause**: `WithCause` attaches the technical error. It is reported in the log
  and never to the user.

`domain.NewValidationErrf` stays the short form for the validation of input.

The status code is derived from the kind:

| Kind                                            | Status                    |
|-------------------------------------------------|---------------------------|
| `ErrValidation`, `ErrInvalidArgument`           | 400 Bad Request           |
| `ErrConstraintViolation`                        | 400 Bad Request           |
| `ErrOperationNotPermitted`                      | 400 Bad Request           |
| `ErrNotFound`                                   | 404 Not Found             |
| `ErrNotAuthenticated`                           | 401 Unauthorized          |
| `ErrNotAuthorized`                              | 403 Forbidden             |
| `ErrRetryable`                                  | 503 Service Unavailable   |
| Error of an Incus server                        | 502 Bad Gateway           |
| everything else                                 | 500 Internal Server Error |

## Errors not caused by the user

Everything, that is not classified, is an internal error. The user only gets
`Internal server error, see the Operations Center log for details` together with
the request ID, the error itself stays in the log. Such errors only need enough
context to be understood in the log:

```go
return fmt.Errorf("Failed to fetch servers of cluster %q: %w", name, err)
```

## Wrapping

* Wrap with `%w`, never with `%v`, so the kind of the error survives.
* Only wrap, if the wrap adds context, which is not there yet. Context, that an
  inner layer already carries, is not repeated.
* A `Failed to <operation>` prefix of a handler or a service is for the log
  only. It is not reported to the user, as long as the error is classified, so
  it never has to carry the message the user needs.

The message for the user is picked by `domain.UserMessage`: it returns the
message of the outermost `domain.Error` of the chain, or, for an error, that is
only classified by a wrapped kind, the message of the chain without the text of
the kind.

## Reasons

A reason is a stable, machine readable identifier for a concrete error
condition, declared in `shared/api/error.go` in lower snake case, e.g.
`server_not_evacuated`. Clients branch on it, so it never changes once it is
released.

Add a reason only, if a client needs to tell the condition apart from other
errors of the same kind, e.g. to offer an action. Without a reason, the generic
reason of the kind is reported, e.g. `not_found`.

## Hints

A hint tells the user how to resolve the error. It is part of the response, so
every client reports the same guidance:

* Write it as a full sentence with a trailing period.
* Keep it client agnostic. `Evacuate the server first or use the force option.`
  works for the CLI, the UI and any other client, `Run operations-center …` does
  not.
* Name the option, which lifts the restriction, if there is one.

Client specific guidance, e.g. a concrete command or a button, is mapped by the
client from the reason. Prose is never duplicated there.

## Logging

* **Log or return, never both.** An error, that is returned to the caller, is
  logged by the boundary it ends up at.
* The boundaries are the response middleware (`internal/util/response`), which
  logs one record per failed request, the background tasks, the authentication
  middleware and the goroutines, which have no caller left to report to.
* Errors are logged with `logger.Err`, so they always use the `err` key.
* The decorators generated for the services, repositories and ports only trace
  returned errors at debug level. Raise the level of the component to see them,
  see `settings.log_levels`.

A record of a failed request carries the message reported to the user
(`response`), the reason, the details and the complete chain (`err`):

```
WRN Request failed method=DELETE request_uri=/1.0/provisioning/servers/server01
    status_code=400 response="Server \"server01\" is a member of cluster \"one\" and can not be deleted"
    reason=server_is_cluster_member details="map[cluster:one server:server01]"
    err="Failed to delete server \"server01\": Server \"server01\" is a member of cluster \"one\" and can not be deleted"
    component=api request_id=3f1c1a4e-0a0e-4a5f-9a1e-2d7b8c9e0f11
```

## Errors recorded on entities and warnings

Fields, which keep an error for the user, e.g. the last error of a deployment or
of a cluster update, store `domain.UserMessage(err)`. The technical details are
logged where the error is recorded.

A warning, which reports an error, is created with
`warning.NewWarningFromError`. It stores the message for the user and lets
`Emit` log the error itself:

```go
s.warning.Emit(ctx, warning.NewWarningFromError(api.WarningTypeUnreachable, scope, err, "Server is unreachable"))
```

## Testing

Assert on the kind, the reason and the hint, not on the prose of a message, so
an improved message does not break the tests. The helpers are in
`internal/util/testing/errassert`:

```go
assertErr: func(tt require.TestingT, err error, a ...any) {
    errassert.DomainError(domain.ErrOperationNotPermitted, api.ErrorReasonServerNotEvacuated)(tt, err, a...)
    errassert.HintIs("Evacuate the server first or use the force option.")(tt, err, a...)
},
```

* `errassert.DomainError(kind, reason)` asserts the kind and the reason at once,
  which is what a client reacts to.
* `errassert.ReasonIs` and `errassert.HintIs` assert one of the two on their own.
* `errassert.UserMessageContains` asserts on the message, where the prose is the
  point of the test, e.g. where two conditions are told apart. Use the shortest
  fragment, which distinguishes them, not the whole sentence.

## Enforcement

`make lint` runs `cmd/domain-errors`, which reports the places, that do not
follow these conventions, and fails the build. Run it on its own with:

```sh
go run ./cmd/domain-errors ./...
```

It resolves the kinds and the reasons through the type information, so it sees
them through an import alias and through a reference inside the `domain` package
itself.

### Layers

Two rules depend on which layer the code belongs to, derived from the packages
themselves:

* **Infrastructure** is a package below a `repo` or an `adapter` directory, and
  everything under `internal/adapter` and `internal/sql`. It talks to the
  database, the file system or another server.
* **Business logic** is a package holding a `*_service.go`. That covers the
  files next to the service as well, e.g. `server_deployment.go`, and a new
  service package is in scope without anybody adding it anywhere.

Every other package is left alone. The remaining rules only fire where a kind is
referenced or a user facing error is built, so a package, which does neither,
never produces a finding.

### Rules

| Rule                                                                                         | What it reports                                                                                                                                                                                                                                                                                                        |
|:---------------------------------------------------------------------------------------------|:-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `kind-without-domain-error`                                                                  | `fmt.Errorf` or `errors.Join` receives a kind, which classifies the error without a reason, a hint or details. `errors.Is` and `errors.As` are not reported, they identify an error rather than build one.                                                                                                             |
| `bare-kind-escapes`                                                                          | A kind is returned as the error itself, outside the infrastructure layer. It carries no message, so the user reads the text of the kind alone, e.g. `Not found`.                                                                                                                                                       |
| `error-without-kind`                                                                         | An error, which originates in the business logic, carries no kind, so nothing says whether it is meant for the user. A wrap is not reported, it inherits the kind of what it wraps, and neither is an error handed to `domain.NewRetryableErr`, `domain.NewValidationErrf` or a response helper setting a status code. |
| `unclassified-boundary-error`                                                                | `response.SmartError` reports an error, which is created right there without a kind, so the user only gets the generic internal server error. A helper forcing a status code of its own, e.g. `response.BadRequest`, is not reported.                                                                                  |
| `message-not-capitalized`, `message-trailing-period`, `message-empty`, `message-wraps-error` | The message does not read the way a message for the user is meant to read.                                                                                                                                                                                                                                             |
| `hint-not-capitalized`, `hint-no-full-sentence`, `hint-empty`                                | The hint is not a full sentence.                                                                                                                                                                                                                                                                                       |
| `hint-client-specific`                                                                       | The hint names one particular client, although it is reported to every client.                                                                                                                                                                                                                                         |
| `reason-not-declared`                                                                        | The reason is not an `api.ErrorReason` of `shared/api`. A reason resolved by a caller, e.g. a parameter of a helper, is accepted.                                                                                                                                                                                      |
| `detail-key-not-constant`, `detail-key-not-snake-case`                                       | A detail is not named the way the log names its attributes.                                                                                                                                                                                                                                                            |
| `cause-reclassifies-error`                                                                   | `WithCause` attaches a kind. `errors.Is` finds the cause as well, so the cause, not the kind of the error, decides the status code.                                                                                                                                                                                    |
| `reason-unused`                                                                              | A reason is declared in `shared/api` but never used, so it never reaches a client.                                                                                                                                                                                                                                     |
| `unknown-domain-construct`                                                                   | `internal/domain` gained a constructor or a `With` method of `domain.Error`, which the checker does not read, so the errors built with it would pass every rule unchecked.                                                                                                                                             |

### Directives

An error, which is internal by design, e.g. a violated invariant the user can do
nothing about, stays unclassified and says so. It is reported as an internal
server error, which is what it is:

```go
//domain-errors:internal Programmer error, the BMC API type is not handled.
return fmt.Errorf("Failed to get BMC server client for type %q", server.BMCConfig.APIType)
```

A place, which is right the way it is although a rule says otherwise, carries the
general escape hatch instead, which says why:

```go
//domain-errors:ignore The kind is attached for the caller, which classifies it.
return fmt.Errorf("...: %w", domain.ErrNotFound)
```
