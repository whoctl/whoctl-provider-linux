---
subcategory: Packages
---

# AptPackage

A package managed by Debian's `apt`. The object's name is the package name; the
spec says whether it should be there and, optionally, at which version.

There is one kind per package manager rather than a single `Package`, because
packages are the part of a system that does not travel: the same software is
`openssh-server` here and `openssh` under apk, and the two version strings are
written in different grammars. A manifest that says `AptPackage` cannot be
mistaken for one that would work on Alpine.

## Example

```yaml
apiVersion: linux.whoctl.io/v1alpha1
kind: AptPackage
metadata:
  name: tree
spec:
  state: installed
```

```console
$ whoctl apply -f tree.yaml
linux/aptpackage/tree created

$ whoctl get linux/apt tree -o wide
NAME   STATE       VERSION   ARCH    ORIGIN   DESCRIPTION
tree   installed   2.2.1-1   amd64   -        displays an indented directory tree, in color

$ whoctl get linux/apt tree -o yaml | whoctl apply -f -
linux/aptpackage/tree unchanged

$ whoctl delete linux/apt tree
linux/aptpackage/tree deleted
```

A pinned version is written exactly as dpkg records it, epoch and revision
included:

```yaml
spec:
  state: installed
  version: 2.2.1-1
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

**Reads come from dpkg, writes go through apt.** Installed packages are parsed
out of `/var/lib/dpkg/status`, the record of what is actually unpacked and
configured. That file also lists packages that are merely known, or already
removed, so only a `Status` of `install ok installed` counts. Mutations run
`apt-get` rather than `dpkg`, so dependencies are resolved instead of refused.

**`remove`, never `purge`.** Deleting a package leaves its configuration under
`/etc` in place. Configuration belongs to the administrator, not to the package,
and throwing it away is not a decision `delete` should make on its own.

**apt is never asked a question.** Commands run with `-y` and
`DEBIAN_FRONTEND=noninteractive`; without it a package carrying a debconf prompt
would block forever on a dialog nobody is there to answer.

**An omitted version means "any version", not "the newest".** An uninstalled
package is installed at whatever the configured sources offer; an installed one
is left alone, so `apply` does not become an upgrade every time it runs.

**`status.origin` is usually empty.** dpkg does not record which repository a
package came from, so unlike apk and rpm there is nothing honest to report.

**This kind needs apt-get.** On a machine without it every verb fails with
`apt is not available on this system` instead of reporting an empty list.

See [AptRepository](aptrepository.md) for the sources apt installs from.
