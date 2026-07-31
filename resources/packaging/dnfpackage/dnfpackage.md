---
subcategory: Packages
---

# DnfPackage

A package managed by `dnf`, or by `yum` on the older Red Hat family. The
object's name is the package name; the spec says whether it should be there and,
optionally, at which version.

The kind drives whichever of the two binaries the machine has — `dnf` when it is
present, `yum` otherwise — because the command line whoctl uses is the same on
both. That is a difference between two front ends to one package format, not
between two package managers, so it does not deserve two kinds.

## Example

```yaml
apiVersion: linux.whoctl.io/v1alpha1
kind: DnfPackage
metadata:
  name: curl
spec:
  state: installed
```

```console
$ whoctl get linux/dnf curl -o wide
NAME   STATE       VERSION         ARCH     ORIGIN           DESCRIPTION
curl   installed   8.18.0-7.fc44   x86_64   Fedora Project   A utility for getting files from remote servers (FTP, HTTP, and others)

$ whoctl get linux/dnf curl bash
NAME   STATE       VERSION
curl   installed   8.18.0-7.fc44
bash   installed   5.3.9-3.fc44
```

A pinned version is written the way rpm reports one, version and release joined:

```yaml
spec:
  state: installed
  version: 8.18.0-7.fc44
```

## Spec

<!-- whoctl:begin spec -->
| Field | Type | Notes | Description |
| --- | --- | --- | --- |
| `state` | string | optional | Whether the package should be present: installed or absent. Defaults to installed. Example: `installed`. |
| `version` | string | optional | Exact version to hold the package at, in the manager's own format. Omitted, whoctl installs the newest version the configured repositories offer and then leaves an already-installed package alone. Example: `8.12.1-r0`. |
<!-- whoctl:end spec -->

## Status

<!-- whoctl:begin status -->
| Field | Type | Description |
| --- | --- | --- |
| `state` | string | Whether the package is installed or absent. |
| `version` | string | The version currently installed. Example: `8.12.1-r0`. |
| `architecture` | string | The architecture the installed package was built for. Example: `x86_64`. |
| `description` | string | One-line summary, as recorded by the package manager. |
| `origin` | string | Where the package came from, when the manager records it: the source package under apk, the vendor under rpm. |
| `manager` | string | The package manager this kind drives. Example: `apk`. |
<!-- whoctl:end status -->

## Columns

<!-- whoctl:begin columns -->
| Column | Shown |
| --- | --- |
| `NAME` | always |
| `STATE` | always |
| `VERSION` | always |
| `ARCH` | `-o wide` |
| `ORIGIN` | `-o wide` |
| `DESCRIPTION` | `-o wide` |
<!-- whoctl:end columns -->

## Behaviour

**This is the one kind that reads through a command.** Everywhere else the
provider parses the system's own files, but rpm keeps its database in a binary
store — sqlite today, Berkeley DB before that — with no stable on-disk format to
read. `rpm -qa` with an explicit query format is both the supported interface
and the one that will still work after the next rpm release.

**An omitted version means "any version", not "the newest".** An uninstalled
package is installed at whatever the enabled repositories offer; an installed
one is left alone, so `apply` does not turn into an upgrade every time it runs.

**`status.origin` is the rpm vendor**, which is as close as rpm comes to
recording where a package came from. It is the packager's name — `Fedora
Project` — not the repository id.

**`gpg-pubkey` entries show up in listings.** rpm stores imported signing keys
in the same database as packages, so they appear in `get linux/dnfpackages`.
They are genuinely in the database, so whoctl reports them rather than filtering
the listing into something that no longer matches `rpm -qa`.

**This kind needs dnf or yum.** On a machine with neither, every verb fails with
a message naming the missing binary rather than reporting an empty list.

See [DnfRepository](dnfrepository.md) for the repository definitions under
`/etc/yum.repos.d`.
