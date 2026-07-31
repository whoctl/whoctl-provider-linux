---
subcategory: Packages
---

# DnfRepository

A repository section in one of the `.repo` files under `/etc/yum.repos.d`. The
object's name is the section id — the `[docker-ce-stable]` header — because that
is the repository's real identity: it is what `dnf --enablerepo` takes, and it
is unique across files. Which file the section sits in is incidental, and
reported in `status.file`.

## Example

```yaml
apiVersion: linux.whoctl.io/v1alpha1
kind: DnfRepository
metadata:
  name: example
spec:
  displayName: Example
  baseurl: [https://rpm.example.com/fedora]
  gpgcheck: true
  gpgkey: https://rpm.example.com/gpg
```

```console
$ whoctl apply -f example.yaml
linux/dnfrepository/example created

$ cat /etc/yum.repos.d/example.repo
[example]
name=Example
baseurl=https://rpm.example.com/fedora
gpgkey=https://rpm.example.com/gpg
enabled=1
gpgcheck=1

$ whoctl get linux/dnfrepo
NAME                    ENABLED   GPGCHECK   DISPLAYNAME
fedora                  true      true       Fedora $releasever - $basearch
fedora-cisco-openh264   true      true       Fedora $releasever openh264 (From Cisco) - $basearch
```

## Spec

<!-- whoctl:begin spec -->
| Field | Type | Notes | Description |
| --- | --- | --- | --- |
| `enabled` | boolean | optional | Whether dnf should use this repository. Defaults to true, matching dnf's own reading of a missing enabled key. |
| `displayName` | string | optional | The human-readable name, written as the repository's name= key. Example: `Docker CE Stable`. |
| `baseurl` | list of string | optional | One or more archive URLs. Give this, a metalink or a mirrorlist. Example: `https://download.docker.com/linux/fedora/$releasever/$basearch/stable`. |
| `metalink` | string | optional | A metalink URL, as Fedora's own repositories use. |
| `mirrorlist` | string | optional | A mirrorlist URL. |
| `gpgcheck` | boolean | optional | Whether package signatures are verified. Left alone when omitted. |
| `gpgkey` | string | optional | Where the signing key lives. Example: `https://download.docker.com/linux/fedora/gpg`. |
| `file` | string | optional, create-only | Which .repo file to write the section into. Defaults to <name>.repo and is only read when the repository is created. Example: `docker-ce.repo`. |
<!-- whoctl:end spec -->

## Status

<!-- whoctl:begin status -->
| Field | Type | Description |
| --- | --- | --- |
| `enabled` | boolean | Whether dnf uses this repository. |
| `displayName` | string | The repository's name= key. |
| `baseurl` | list of string | The archive URLs. |
| `metalink` | string | The metalink URL, when the repository uses one. |
| `mirrorlist` | string | The mirrorlist URL, when the repository uses one. |
| `gpgcheck` | boolean | Whether package signatures are verified. |
| `gpgkey` | string | Where the signing key lives. |
| `file` | string | The .repo file holding the section. Example: `/etc/yum.repos.d/fedora.repo`. |
<!-- whoctl:end status -->

## Columns

<!-- whoctl:begin columns -->
| Column | Shown |
| --- | --- |
| `NAME` | always |
| `ENABLED` | always |
| `GPGCHECK` | always |
| `DISPLAYNAME` | always |
| `URL` | `-o wide` |
| `FILE` | `-o wide` |
<!-- whoctl:end columns -->

## Behaviour

**Unmodelled keys are preserved exactly where they were.** A real `.repo` file
carries `skip_if_unavailable`, `countme`, `metadata_expire`, module hints and
whatever else the vendor put there. whoctl edits the keys it models and copies
the rest through untouched, in their original order — rewriting the section from
the model alone would silently drop all of it.

**A missing `enabled` key means enabled**, which is how dnf itself reads it, so
that is what `get` reports and what an omitted `spec.enabled` leaves in place.

**`gpgcheck` is left alone when omitted.** Unlike `enabled` it has no safe
default to assume: writing `gpgcheck=0` into a repository that never mentioned
it would turn off signature checking as a side effect of an unrelated change.

**One of `baseurl`, `metalink` or `mirrorlist` is required.** Fedora's own
repositories use a metalink and third-party ones usually use a baseurl; a
section with none of the three points nowhere, so the manifest is refused.

**`spec.file` only applies at creation.** A new section is written to
`<name>.repo` unless the manifest names another file; an existing one is edited
where it already lives, so applying never moves a section between files behind
your back.

**Deleting the last section removes the file.** A `.repo` file left holding
nothing is still read by dnf and is just litter, so it goes; a file with other
sections in it is rewritten without the deleted one.

See [DnfPackage](dnfpackage.md) for the packages these repositories provide.
