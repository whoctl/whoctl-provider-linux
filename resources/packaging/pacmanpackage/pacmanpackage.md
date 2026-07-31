---
subcategory: Packages
---

# PacmanPackage

A package managed by Arch's `pacman`. The object's name is the package name, and
the spec says whether it should be there.

This is the kind where the case for one kind per manager is most visible:
pacman has no way to install a chosen version at all, so `spec.version` — a
field the other three honour — is something this kind has to refuse. A single
shared `Package` kind would have had to either pretend the field worked or drop
it for everyone.

## Example

```yaml
apiVersion: linux.whoctl.io/v1alpha1
kind: PacmanPackage
metadata:
  name: tree
spec:
  state: installed
```

```console
$ whoctl apply -f tree.yaml
linux/pacmanpackage/tree created

$ whoctl get linux/pacman tree -o wide
NAME   STATE       VERSION   ARCH     ORIGIN   DESCRIPTION
tree   installed   2.3.2-1   x86_64   -        A directory listing program displaying a depth indented list of files

$ whoctl get linux/pacman tree -o yaml | whoctl apply -f -
linux/pacmanpackage/tree unchanged

$ whoctl delete linux/pacman tree
linux/pacmanpackage/tree deleted
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

**`spec.version` is refused, not ignored.** pacman installs whatever version the
synced repositories currently hold and offers no syntax for asking for another;
downgrading means reaching into the package cache by filename, which is a
different operation with different failure modes. A manifest that pins a version
therefore fails outright:

```console
$ whoctl apply -f pinned.yaml
error: linux/pacmanpackage/tree: pacman cannot install a chosen version; remove spec.version
```

**Which is why `get` does not export a version here.** Everywhere else `get`
fills `spec` with the observed state so that `get -o yaml | apply -f -` reports
`unchanged`. Exporting `spec.version` under pacman would produce a manifest its
own machine rejects, so the field is left out of the spec and reported in
`status` only. The round-trip still holds.

**Reads parse pacman's local database.** Each installed package is a directory
under `/var/lib/pacman/local` holding a `desc` file of `%FIELD%` headers. A
directory without a readable `desc` is a half-written entry and is skipped
rather than failing the whole listing.

**Installs use `--needed`,** so a package that is already present is not
reinstalled, and `--noconfirm`, so pacman never stops to ask.

**Official repositories only.** The AUR is not covered: an AUR helper builds
from source as an unprivileged user against a PKGBUILD nobody has reviewed,
which is a different trust model from installing a signed binary package. If it
is ever supported it will be as its own kind, for the same reason apk and pacman
are separate kinds today.

**This kind needs pacman.** On a machine without it every verb fails with
`pacman is not available on this system` instead of reporting an empty list.

See [PacmanRepository](pacmanrepository.md) for the repositories in
`/etc/pacman.conf`.
