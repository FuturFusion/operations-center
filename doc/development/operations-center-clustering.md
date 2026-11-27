# Operations Center Clustering

This page lists the rules for code that runs in an Operations Center cluster.
Follow them when you change Operations Center.

"Must" marks a hard rule. If you break it, the members become inconsistent or
the cluster breaks. "Should" marks a strong recommendation. You may deviate
from it if you have a good reason.

For the user view of a cluster, see {doc}`/reference/high-availability`.

## Overview

Operations Center stores its data in cowsql. Cowsql is SQLite, replicated with
the Raft consensus protocol. A standalone server is a cluster with one member.
The same code runs in both cases.

* **Global database:** Cowsql replicates the global database to all members.
  It holds all domain data, the members, the shared configuration and the
  cluster wide patches.
* **Local database:** Each member has a plain SQLite database in
  `var/database/local.db`. It holds the Raft nodes and the member patches.
* **Local configuration:** `config.yml` holds the configuration of one member
  only.
* **Leader:** One member is the Raft leader. Other members forward changes to
  the leader. Only the leader runs the background tasks that
  change shared data.
* **Member files:** Files such as images, updates and cluster artifacts are
  stored on one member. The database records which member stores them. Other
  members forward requests for these files to that member.
* **Certificates:** Each member has its own server certificate. Members use it
  to talk to each other. All members present the shared cluster certificate to
  clients and to managed servers.

The code is in these packages:

| Package                            | Content                                         |
|------------------------------------|-------------------------------------------------|
| `internal/cluster`                 | Membership, leader tasks, notifications.        |
| `internal/cluster/memberfiles`     | Files stored on one member.                     |
| `internal/sql/transaction`         | Retried transactions.                           |
| `internal/sql/dbschema`            | Schema of the global and the local database.    |
| `internal/sql/patches`             | Patches per cluster and per member.             |
| `internal/config/daemon`           | Shared and local configuration.                 |
| `internal/api` (`forward.go`)      | Forwarding to the leader and to a member.       |
| `cluster_tests`                    | Tests with several real members.                |

## Database

### Transactions are retried

`transaction.Do`, `transaction.DoAndReturn` and `transaction.ForceTx` retry the
whole function on a busy database, a lost connection or a leader change. On
the global database, a function can run up to 250 times. Transactions on the
local database are not retried. A nested call does not start its own
transaction. Only the outermost call commits and retries.

* Every repository method must run its statements in `transaction.Do`,
  `transaction.DoAndReturn` or `transaction.ForceTx`. Get the database handle
  inside the function with `transaction.GetDBTX`. A statement outside a
  transaction is never retried. It fails on a busy database.
* For generated inventory repositories, change the template in `cmd/generate-inventory`
  and run `go generate ./...`.
* The function must not have side effects. Each attempt must start from the
  same state. Do not do any of these inside the function:
   * create, delete or rename files;
   * call managed servers, other members or other HTTP services;
   * emit lifecycle signals;
   * start goroutines;
   * change in memory caches.
* Run such actions after `transaction.Do` returns, or register them with
  `transaction.AfterCommit`. `AfterCommit` runs the action once after the
  commit. It drops the action on a rollback or a retry. Outside a
  transaction, `AfterCommit` runs the action at once.
* Reset every variable that the function fills at the start of each attempt.
  Never append to a slice from outside the function.
* An `AfterCommit` action must use `logger.DetachedContext`. The request
  context can be canceled. The action must still run.
* Transactions should be short. The global database has one connection. An
  open transaction blocks all other access to the global database on this
  member.
* Do not use the local and the global database in the same transaction. The
  second database gets the transaction of the first one.

```go
var removed provisioning.Updates
err := transaction.Do(ctx, func(ctx context.Context) error {
    // Do retries this function. Start each attempt with an empty list.
    removed = nil

    for _, update := range updates {
        err := s.repo.DeleteByUUID(ctx, update.UUID)
        if err != nil {
            return err
        }

        removed = append(removed, update)
    }

    return nil
})
if err != nil {
    return err
}

// Delete the files only after the commit. A rollback can not restore them.
for _, update := range removed {
    err = s.files.Delete(ctx, update)
    ...
}
```

```go
// The files are only deleted once the deletion of the image is committed.
return transaction.AfterCommit(ctx, func(ctx context.Context) error {
    ctx = logger.DetachedContext(ctx)

    return s.filesRepo.Delete(ctx, &img)
})
```

Use `internal/util/testing/retrytx` to test that a function survives a retry.
Return `errors.New("database is locked")` from a mock to trigger the retry.
These tests must not call `t.Parallel`, because the transactor is global.

### SQL that cowsql supports

* SQL must not use `RETURNING`. Run the `INSERT` or `UPDATE`, then read the row
  in the same transaction.
* SQL must not use named parameters, such as `:name` or `sql.Named`. Use `?` or
  numbered parameters such as `?1`.
* Large writes must be split into batches. Cowsql fails on one `Exec` with a
  very large number of statements.
* Map database errors with `sqlite.ConstraintErr` or `sqlite.MapErr`. Cowsql
  returns `driver.Error`, not `sqlite3.Error`.
* Code must not read or write the files of the global database. For the same
  reason, do not add file based database backups. Raft replicates the global
  database, so a file copy of one member does not match the other members.
* A schema update that turns off foreign keys must turn them on again on the
  same connection. Use a `defer` for this.

### Schema updates

Members with different versions can run at the same time during an upgrade. A
member with a newer schema waits until all members have the new version.

* Append a new update function to the `updates` list in
  `internal/sql/dbschema`. Never change, remove or reorder an existing update.
* Change `schema/000001_freshschema.sql` to match the result of all updates.
* An update must only change the database. It must not call services or the
  API.
* Defaults must make existing rows valid. For example, `member_id` defaults to
  `1`, the first member.
* Add a `TestSchemaEnsure_...` test with existing rows.
* A new entity package must add a `PrepareStmts` call for its entities package
  to `prepare()` in `internal/api/daemon.go`. A statement that is not prepared
  fails only at runtime.
* Do not change the import from the old SQLite database or the old updates
  without running `TestUpgradeFromSQLite`.

### Patches

Patches in `internal/sql/patches` have a scope.

* Use `scopeCluster` for a patch that changes the global database or the shared
  configuration. It runs once in the cluster.
* Use `scopeMember` for a patch that changes the files of a member. It runs
  once on every member.
* Append new patches. Never reorder or remove a patch.
* A member patch must also work on a fresh member.

### State must survive a leader change

* State that other members need must be in the global database. It must not be
  only in memory or only in a file on one member.
* A file on one member is fine if the database records the member. See
  [Files stored on one member](#files-stored-on-one-member).
* Some state of the leader is in memory, for example the progress of a rolling
  update. A `GET` that returns such state must forward to the leader. See
  [API endpoints](#api-endpoints).

## API endpoints

Every member serves the full API. The `requireLeader` middleware in
`internal/api/daemon.go` forwards every request to the leader. Exceptions:

* `GET` and `HEAD` requests;
* paths in `memberLocalPaths`;
* the update of the own server record of a member through the Unix socket.

Rules:

* A new endpoint that changes data needs no extra code. The middleware forwards
  it to the leader.
* An endpoint that changes data on the receiving member only must be added to
  `memberLocalPaths`. Examples are the local configuration and the join of a
  member. The match is on the exact path.
* A `GET` that returns state from the memory of the leader must call
  `forward.toLeader`.
* An endpoint that reads or writes files stored on one member must call
  `forward.toMember` with the member of the entity. It must do this before it
  reads the request body. Otherwise the forwarded request has an empty body.
* A handler that checks the client certificate or the client address must use
  `clientCertificate`, `CtxForwardedCertificate` or `CtxForwardedAddress`. It
  must not read `r.TLS.PeerCertificates`. After forwarding, the TLS peer is the
  forwarding member, which has member trust.
* A route that touches the database must be behind `requireDatabase`. Before
  the database is open, the database handle is `nil`.
* A route that members need while they start must not use
  `authn.WithRefuseWhileStarting`. The internal API and the IncusOS API stay
  available during startup. Members need them to open the database.
* A wrapper of `http.ResponseWriter` must implement `http.Hijacker`. The cowsql
  gateway hijacks connections between members.
* Do not write headers before the forwarding is done.

## Background tasks

Background tasks change shared data or talk to managed servers. Only the leader
runs them. After a leadership change, the new leader starts each task after the
other members stopped it, or after 30 seconds at most.

* A periodic task must be registered in `backgroundTasks` with
  `d.leaderTask`. Its name must be stable, because the members compare the
  names during the handover.
* A task that only cleans the local cache of a member goes into
  `memberTasks`. It runs on every member.
* Work that a request or a signal starts on the leader must use a leader
  context. The daemon provides it with `d.leaderContext`. Handlers use the
  `leaderContext` function they get when they are registered. The work must
  call the returned function when it is done.
  This ties the work to the term of the leader, so a new leader waits for it.
* Goroutines started by a request must stop on shutdown. Do not use a plain
  `go` statement. Track them, as `runInBackground` of the server service does.
* A task must stop when its context is canceled. Listeners that a task adds
  must be removed on `ctx.Done()`. Otherwise a new term adds the listener
  again.
* The leader starts a task once per term. A task that can fail to start must
  retry by itself.
* Use `BeforeStart` to clean up state of a former leader before a task starts.

```go
ctx, done, ok := u.leaderContext(r.Context())
if !ok {
    return response.SmartError(domain.NewErrorf(domain.ErrUnavailable, "", "This member is not the cluster leader"))
}

go func() {
    defer done()

    err := u.service.Refresh(ctx)
    ...
}()
```

## Configuration

The configuration has two parts:

* **Shared configuration** (`SharedConfig`) is stored as one JSON row in the
  global database. It is the same on all members. Examples are security,
  updates, the cluster settings and `network.address`.
* **Local configuration** (`localConfig`) is stored in `config.yml`. It is only
  for one member. Examples are the listen addresses and the logging, metrics
  and pprof settings.

A change of the shared configuration is stored in the database, then every
member reloads it. If an online member can not apply the change, the change is
reverted. A member that is offline applies the stored configuration when it
starts. Only the changed section is written. This way a member that missed a
change can not overwrite it.

### Add a shared configuration option

1. Decide if the option is shared or local. An option must be shared if all
   members must use the same value.
1. Add the field to the type in `shared/api/system`.
1. Extend the change check in `internal/config/daemon/changes.go`.
1. Add the default in `normalizeShared`.
1. Add the validation in `validate.go`.
1. Add the notification in `notify.go`. Use `AddListenerWithErr`, so a failed
   listener reverts the change.
1. Add a migration in `legacy.go`, if the option existed in an older format.
1. Update the inventory templates in `cmd/generate-inventory` and
   `cmd/populate-db`.
1. For a new section, add the section constant, the getter, the updater and
   the ETag section. The API needs `GET` and `PUT` with an ETag and a `412`
   response. Update the client, the CLI, the UI and the API documentation
   (`make update-api`).
1. Update the stored format test data of `TestConfigRepo_storedFormat`.

### Rules

* Never change an existing JSON tag of `SharedConfig` or its sections. The tags
  are the stored format.
* An older member does not know a new field. During a rolling upgrade, it can
  drop the field when it writes the section. A new field must work if it is
  dropped this way, or the change needs a new schema version. The code does not
  check this yet.
* A `PUT` of the shared configuration must pass `If-Match` with
  `config.WithIfMatch`. The `GET` must compute the ETag from the shared part
  only. Otherwise concurrent changes through different members are lost.
* A listener of a shared signal runs on every member. It must be idempotent. It
  must return an error if it can not apply the change.
* If only one member must act on a change, the listener must check that it runs
  on the leader. An example is the ACME certificate renewal.
* A listener must not react to the initial load of the configuration
  (`sectionLoaded`) as if it were a change.
* Validation functions and listeners may read the configuration. They must not
  change it. A change from inside them blocks the configuration update forever.
* A new lifecycle signal must be reset in `Daemon.Stop`.

## Files stored on one member

Images, update files and cluster artifacts are stored on the member that
created or downloaded them. The column `member_id` records the member. A
standalone server uses `memberfiles.StandaloneMemberID`.

### Add a file based entity

1. Add a `member_id` column with a foreign key to `cluster_members`. Existing
   rows default to `1`.
1. Set `MemberID` to the local member when you create the entity. Get the local
   member through a port.
1. Serve the files through `forward.toMember`.
1. Access the files on other members through an adapter based on
   `memberfiles.Targets`. The adapter implements a port of the service, so the
   service stays pure.
1. Map an unreachable member to `domain.ErrUnavailable`. Do not return "not
   found" for files on an unreachable member.
1. Implement `memberfiles.FileTargetRepo` and register it in `NewMover`. Then
   moving files and removing members works for the new entity.
1. Add the store to `DeleteStores`. Add the known files to `pruneLocalFiles`.
1. Create, copy and delete files outside of the transaction, or with
   `transaction.AfterCommit`.
1. Add a cluster test. See [Testing](#testing).

### Rules

* Move a member's files before normal removal. Forced removal bypasses removal
  checks. Register additional removal checks as `RemovalCheck` values in
  `memberRemovalChecks`.
* A loop over entities on several members should skip unreachable members with
  a warning. The member deletes stale files on its next startup.
* A cache file that a member can generate again should be recorded with
  `mediarecord` and a new media type. Renew the lease when the file is read.
  Prune by the records, not by the file time alone. Otherwise members delete
  the files of each other.
* Store files by a stable key, such as a UUID. A rename must not move files.

## Certificates and trust

* Code that needs the certificate of Operations Center must use the cluster
  certificate when clustered (`operationsCenterCertificate`). Never read
  `server.crt` from disk for this.
* Code that identifies a member must use the member certificate
  (`IsMemberCertificate`). Only trust the leaf certificate.
* A change of a certificate on all members must follow the policy of
  `UpdateClusterCertificate`. With the same key, try all members and skip
  offline members. With a new key, all members must be online. Each member
  gets a revert.
* Code that adds trusted members must refresh the certificate cache.
  Otherwise the new member gets a `403` response.
* Members share one client key pair for managed servers. Replace it through the
  key pair holder, not by writing files.

## Membership, startup and shutdown

* An operation that changes Raft roles or the member list must hold `rolesMu`.
* An internal call that only the leader may handle must check the leadership
  or forward to the leader.
* Add a new check that blocks a join to `joinCheck` in
  `internal/api/membership_join.go`.
* Every blocking wait during startup must watch the context. Opening the
  database can wait a long time for other members.
* The listeners start before the database. Code that runs at startup must not
  expect the database before it is registered.
* A server action that takes a member host down must call
  `checkOperationsCenterQuorum`. This applies to power off, reboot and update.
  Actions on member server records must respect
  `refuseOperationsCenterMember`.

## Rollback and detached contexts

Most changes are forwarded to the leader. The client or the forwarding member
can close the connection during the change. Then the request context is
canceled.

* Every revert function must use `context.WithoutCancel(ctx)`. Otherwise the
  revert fails, and managed servers or members stay half changed.
* Work that continues after the request must use a detached context. If it is
  work of the leader, it must also use `d.leaderContext`.

```go
// Rollbacks still run after the client went away.
revertCtx := context.WithoutCancel(ctx)

reverter.Add(func() {
    err = s.repo.Update(revertCtx, *previousCluster)
    ...
})
```

## Concurrency

* Replace the authenticator and the authorizer with `Replace`. Never assign
  them directly. Other members reload the security configuration while the
  member serves requests.
* Package level response values must not change. `Render` must not change its
  receiver.
* Do not add logging middleware to the transactor or the Raft repositories. They
  run for every transaction.

## Build and dependencies

* The daemon needs the raft and cowsql libraries. Run `make deps` once. Use
  the build tag `libsqlite3`, as the `Makefile` does.
* The CLI builds without cgo. Packages that the CLI imports must not need cgo.
* IncusOS does not yet ship the libraries. The end to end tests copy them into
  the virtual machine.

## Testing

* Repository tests run against a real cowsql node (`cowsqltest.Open`), not
  against plain SQLite. This finds SQL that cowsql does not support.
* `cluster_tests` starts several real members. Run it with
  `make cluster-test UI=0 CLUSTER_TEST_LEGACY_VERSION=v0.11.0`. Select tests
  with `CLUSTER_TEST_RUN`. A short `TMPDIR`, such as `/var/tmp`, avoids a too
  long Unix socket path.
* A new feature must have a cluster test that uses it through a member that is
  not the leader.
* A file based entity must have a cluster test for these cases:
   * The files are available through every member.
   * The request fails with "unavailable" while the member with the files is
     down.
   * Moving and pruning the files works.
* A shared configuration option must have tests for the revert after a failed
  member and for the ETag check.
* A leader task must not run on two members at the same time during a
  handover. Use `watchLeaderTasks` in the cluster test.
* A schema update must have a test with existing rows.

## Checklist

Check these points before you merge a change:

* Database statements run in `transaction.Do`, `DoAndReturn` or `ForceTx`.
* Transaction functions have no side effects and reset their variables.
* The SQL has no `RETURNING` and no named parameters.
* Schema updates are appended, the fresh schema matches, and defaults make
  existing rows valid.
* Patches have the correct scope.
* New endpoints are classified: forwarded to the leader, local to the
  member, or forwarded to the member with the files.
* Handlers read the client identity from the forwarded values.
* Periodic tasks are leader tasks. Work started by requests uses
  `d.leaderContext`.
* Shared configuration changes keep the JSON tags, use ETags and have
  listeners that return errors.
* Files are recorded with their member, served with `forward.toMember`,
  registered in the mover and pruned.
* Reverts use a detached context.
* State that other members need is in the global database.
* `make test` and `make cluster-test UI=0 CLUSTER_TEST_LEGACY_VERSION=v0.11.0`
  pass.
