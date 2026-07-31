---
subcategory: Packages
---

# PacmanRepository

A repository section of `/etc/pacman.conf`. The object's name is the section
header — `[extra]`, `[multilib]` — and the spec says where pacman should fetch
from, either explicit servers or an included mirrorlist.

## Example

```yaml
apiVersion: linux.whoctl.io/v1alpha1
kind: PacmanRepository
metadata:
  name: example
spec:
  servers: [https://arch.example.com/$repo/os/$arch]
  sigLevel: Optional TrustAll
```

```console
$ whoctl get linux/pacmanrepo
NAME            ENABLED   SERVERS                   INCLUDE
core            true      -                         /etc/pacman.d/mirrorlist
core-testing    false     -                         /etc/pacman.d/mirrorlist
extra           true      -                         /etc/pacman.d/mirrorlist
extra-testing   false     -                         /etc/pacman.d/mirrorlist

$ whoctl apply -f example.yaml
linux/pacmanrepository/example created

$ tail -3 /etc/pacman.conf
[example]
SigLevel = Optional TrustAll
Server = https://arch.example.com/$repo/os/$arch
```

## Spec

<!-- whoctl:begin spec -->
| Field | Type | Notes | Description |
| --- | --- | --- | --- |
| `enabled` | boolean | optional | Whether pacman should use this repository. A disabled one is commented out in place, the way the shipped pacman.conf carries multilib. |
| `servers` | list of string | optional | Explicit mirror URLs. Give these or an include. Example: `https://mirror.example/archlinux/$repo/os/$arch`. |
| `include` | string | optional | A mirrorlist file to read servers from, which is how Arch's own repositories are configured. Example: `/etc/pacman.d/mirrorlist`. |
| `sigLevel` | string | optional | Signature policy for this repository. Example: `Required DatabaseOptional`. |
<!-- whoctl:end spec -->

## Status

<!-- whoctl:begin status -->
| Field | Type | Description |
| --- | --- | --- |
| `enabled` | boolean | Whether the section is active or commented out. |
| `servers` | list of string | The mirror URLs listed in the section. |
| `include` | string | The mirrorlist the section includes. |
| `sigLevel` | string | The signature policy in force. |
| `file` | string | The file the section was read from. Example: `/etc/pacman.conf`. |
<!-- whoctl:end status -->

## Columns

<!-- whoctl:begin columns -->
| Column | Shown |
| --- | --- |
| `NAME` | always |
| `ENABLED` | always |
| `SERVERS` | always |
| `INCLUDE` | always |
| `SIGLEVEL` | `-o wide` |
<!-- whoctl:end columns -->

## Behaviour

**`[options]` is not a repository and is never touched.** pacman keeps its own
settings in the same file, in a section that looks exactly like a repository.
It is excluded from listings, and a manifest naming it is refused rather than
allowed to rewrite the machine's package configuration:

```console
$ whoctl apply -f options.yaml
error: linux/pacmanrepository/options: [options] is pacman's own configuration, not a repository
```

**A commented-out section is a disabled repository.** `#[multilib]` with its
body commented out is how the shipped `pacman.conf` carries a repository that is
available but off, so that is what `enabled: false` writes and what `get`
reports. The whole block is commented, not just the header, so enabling it back
restores a working section.

One consequence worth knowing: the commented example block the stock
`pacman.conf` ships with — `#[repo-name]` with a placeholder `Server` — is a
commented section like any other, so it shows up as a disabled repository named
`repo-name`. It is genuinely in the file, so whoctl reports it rather than
special-casing a name that a real repository could also use.

**Either servers or an include is required.** Arch's own repositories use
`Include = /etc/pacman.d/mirrorlist`; a third-party one usually lists `Server`
lines. A section with neither points nowhere and is refused.

**Everything outside a repository section is copied through**, comments and
blank lines included, so the file stays as readable as its author left it.

See [PacmanPackage](pacmanpackage.md) for the packages these repositories
provide.
