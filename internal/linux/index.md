---
title: Overview
---

# Linux provider

The linux provider manages the machine whoctl runs on: the accounts and groups
in `/etc/passwd` and `/etc/group`, the services the init system knows about, the
resolver configuration in `/etc/resolv.conf`, and the packages and repositories
of whichever package manager the distribution uses. Everything is an object with
`apiVersion`, `kind`, `metadata` and `spec`, and everything is read with `get`,
changed with `apply` or `edit`, and removed with `delete`.

Its resources are addressed as `linux/<resource>`, and the provider also answers
to `nix`, so `whoctl get linux/users` and `whoctl get nix/usr` are the same
command.

```yaml
apiVersion: linux.whoctl.io/v1alpha1
kind: User
metadata:
  name: alice
spec:
  uid: 4200
  primaryGroup: developers
  groups: [wheel]
  shell: /bin/sh
  comment: Alice Liddell
```

```console
$ whoctl apply -f alice.yaml
linux/user/alice created

$ whoctl get linux/user alice -o yaml | whoctl apply -f -
linux/user/alice unchanged
```

## How it works

**Reads parse files, writes shell out.** Reading `/etc/passwd` directly is fast
and predictable, and one command parses the file once no matter how many objects
it reports on. Writing goes through `useradd`, `adduser` and friends, so
distribution behaviour — skeleton files, file locking, PAM hooks — is preserved
and `/etc` is never corrupted by a half-written line.

`/etc/resolv.conf` is the exception, because no native tool owns it. whoctl
rewrites it itself, keeping every line it does not model verbatim.

**Two account toolsets.** Alpine ships the BusyBox applets (`adduser`,
`deluser`); everyone else ships shadow-utils (`useradd`, `usermod`). The
provider prefers shadow-utils and falls back to BusyBox. BusyBox has no
`usermod` or `groupmod`, so changing an existing account there fails with an
explanation and a pointer at `apk add shadow` rather than silently doing
nothing.

**Two init systems.** systemd when `/run/systemd/system` exists, OpenRC when
`/etc/init.d` is there *and* OpenRC's own commands are too. The second half of
that test matters: Debian keeps sysvinit scripts in `/etc/init.d` without any of
OpenRC's tooling, and matching on the directory alone would hand it a backend
whose every mutation fails with `rc-service: not found`. Under OpenRC nothing is
spawned to read state at all — what exists, what is enabled and what is running
are all filesystem lookups.

**Four package managers, four kinds each.** apk, apt, dnf and pacman get a
package kind and a repository kind apiece, rather than one `Package` covering
them all. Accounts are portable and packages are not: the same software is
`openssh-server` under apt and `openssh` under apk, version strings follow four
grammars, and pacman cannot pin a version at all. A shared kind would produce
manifests that look portable and are not.

Every manager is registered on every machine, so this documentation and
`whoctl resources` say the same thing everywhere. Using a kind whose manager
is not installed is an error that says so, rather than an empty list:

```console
$ whoctl get linux/aptpackages
error: apt is not available on this system: "apt-get" is not in PATH
```

Package databases are read directly, like everything else here — apk, dpkg and
pacman all keep theirs as flat text. rpm is the exception: its database is a
binary store with no stable on-disk format, so the dnf backend queries it
through `rpm`. Repository files are both read and written by whoctl, because no
native tool owns them end to end; unmodelled keys and comments survive a
rewrite, exactly as they do in `/etc/resolv.conf`.

## Requirements

- Writes need root. Reads do not.
- Without permission to read `/etc/shadow`, lock state reports as unlocked
  instead of failing the command.
- `--root` points the reads at another filesystem tree, which is how the tests
  work against fixtures instead of the real machine.

## Behaviour worth knowing

- **`apply` is an upsert.** Missing objects are created, existing ones are
  reconciled, matching ones report `unchanged`.
- **Export round-trips.** `get` fills `spec` with the observed state, so
  `whoctl get linux/user alice -o yaml | whoctl apply -f -` reports `unchanged`.
- **Absent is not empty.** Omitting a list field leaves it alone; an empty list
  empties it. `groups: []` removes every supplementary group.
- **`delete -f` walks the manifest backwards**, so a file that declares a group
  and then its members removes the members first.
- **Credentials are write-only.** `spec.passwordHash` can be applied but is
  never read back, so an exported manifest cannot leak a hash.
