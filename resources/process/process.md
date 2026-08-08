---
subcategory: Runtime
verbs: [get, describe]
---

# Process

A process running on this machine, read from `/proc`.

## Example

```console
$ whoctl get linux/processes
NAME    COMMAND    USER   STATE      RSS      AGE
1       systemd    root   sleeping   12.4M    6d
842     sshd       root   sleeping   8.1M     6d
15310   nvim       naner  sleeping   96.2M    41m
```

## It is the one kind here that is not configuration

Everything else this provider serves is something the machine is *configured*
with — an account, a repository, a resolver line — and configuration is what
`apply` reconciles to. A process is something the machine is *doing*, and there
is no desired state to converge on: it is there or it is not.

So this kind reads, and says so in its verbs rather than accepting an `apply`
and failing afterwards.

## Why `delete` does not kill

`delete` means the same thing everywhere else in whoctl: remove configuration
that can be applied again. Killing a process is neither reversible nor
declarative, and putting it behind the same verb would mean the most destructive
thing the tool can do is one keypress away in a table somebody is browsing.

What a process actually needs is a signal, and a signal is not a state. Use the
tool that owns the process — `systemctl` for a unit, `kill` for the rest.

## Reading it

The pid is the name, so a process is fetched by it:

```sh
whoctl get linux/process 842
whoctl get linux/processes --field-selector status.state=zombie
whoctl get linux/processes --field-selector spec.user=root
```

`AGE` is the process's own: it is the boot time plus the start time the kernel
reports, so it says how long that process has been up rather than how long the
machine has.

## Spec

<!-- whoctl:begin spec -->
| Field | Type | Notes | Description |
| --- | --- | --- | --- |
| `command` | string | **required** | The executable's own name, as the kernel reports it. Example: `sshd`. |
| `args` | list of string | optional | The full command line, argv[0] included. A kernel thread has none. |
| `user` | string | optional | The user the process runs as, resolved from its real uid. Example: `root`. |
<!-- whoctl:end spec -->

## Status

<!-- whoctl:begin status -->
| Field | Type | Description |
| --- | --- | --- |
| `pid` | integer | The process id, same as metadata.name. |
| `ppid` | integer | The parent's process id. |
| `state` | string | running, sleeping, waiting, zombie, stopped or idle. Example: `sleeping`. |
| `threads` | integer | How many threads it has. |
| `memoryRss` | integer | Resident set size in bytes: what it is actually holding in memory. |
| `uid` | integer | The real user id. |
<!-- whoctl:end status -->
