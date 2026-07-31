---
subcategory: System
verbs: [get, describe, apply, edit, restart]
---

# Service

A service the init system knows about: whether it runs right now, and whether it
comes back after a reboot. Both fields are optional, so a manifest can manage
boot behaviour without touching the running state, or the other way round.

## Example

```yaml
apiVersion: linux.whoctl.io/v1alpha1
kind: Service
metadata:
  name: crond
spec:
  enabled: true
  state: running
```

```console
$ whoctl get linux/services -o wide
NAME     STATE     ENABLED   INIT     RUNLEVELS   DESCRIPTION
crond    running   true      openrc   default     Periodic command scheduler
sshd     stopped   false     openrc   -           OpenSSH server

$ whoctl restart linux/service crond
linux/service/crond restarted
```

## Spec

<!-- whoctl:begin spec -->
| Field | Type | Notes | Description |
| --- | --- | --- | --- |
| `enabled` | boolean | optional | Whether the service starts at boot. |
| `state` | string | optional | The state right now: running or stopped. Example: `running`. |
<!-- whoctl:end spec -->

## Status

<!-- whoctl:begin status -->
| Field | Type | Description |
| --- | --- | --- |
| `state` | string | Whether the service is running or stopped. |
| `enabled` | boolean | Whether the service starts at boot. |
| `description` | string | Description taken from the init script or the unit file. |
| `initSystem` | string | The init system behind the service: openrc or systemd. |
| `runlevels` | list of string | OpenRC only: the runlevels the service is enabled in. |
| `unitState` | string | systemd only: the raw enablement word, such as enabled, disabled, static or masked. Example: `enabled`. |
<!-- whoctl:end status -->

## Columns

<!-- whoctl:begin columns -->
| Column | Shown |
| --- | --- |
| `NAME` | always |
| `STATE` | always |
| `ENABLED` | always |
| `INIT` | `-o wide` |
| `RUNLEVELS` | `-o wide` |
| `DESCRIPTION` | `-o wide` |
<!-- whoctl:end columns -->

## Behaviour

**There is no create and no delete.** What defines a service is an init script
or a unit file, which is a package's job. Applying a manifest for a service that
is not installed fails and says so, and `delete` refuses outright rather than
quietly stopping it. To turn a service off, apply `enabled: false` and
`state: stopped`.

**Restart is a verb, not a state.** The state before and after a restart is the
same, so it cannot be expressed as a desired state. It lives on
`whoctl restart linux/service NAME` instead.

**Enablement is applied before the running state**, so a service that is about
to start is already set to come back after a reboot.

**The init system is detected at runtime**: systemd when `/run/systemd/system`
exists, OpenRC otherwise. Under OpenRC, `status.runlevels` lists where the
service is enabled. Under systemd, `status.unitState` keeps the exact word
systemd uses — `enabled`, `static`, `masked` — which carries more nuance than
`status.enabled` can.
