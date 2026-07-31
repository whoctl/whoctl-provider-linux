# whoctl-provider-linux

The linux provider for [whoctl](https://github.com/whoctl/whoctl): accounts,
services, packages, repositories and the resolver, on the local system.

```console
$ whoctl get linux/users
$ whoctl apply -f users.yaml
$ whoctl get linux/user alice -o yaml | whoctl apply -f -
```

Installed on first use; nothing to do by hand.

## Kinds

| Kind | Resource | Backed by |
| --- | --- | --- |
| `User` | `linux/users` | `/etc/passwd`, `/etc/shadow` |
| `Group` | `linux/groups` | `/etc/group` |
| `Service` | `linux/services` | OpenRC or systemd |
| `Nameserver` | `linux/nameservers` | `/etc/resolv.conf` |
| `SearchDomain` | `linux/searchdomains` | `/etc/resolv.conf` |
| `ResolverOption` | `linux/resolveroptions` | `/etc/resolv.conf` |
| `ApkPackage` | `linux/apkpackages` | apk |
| `AptPackage` | `linux/aptpackages` | apt |
| `DnfPackage` | `linux/dnfpackages` | dnf / yum |
| `PacmanPackage` | `linux/pacmanpackages` | pacman |
| `ApkRepository` | `linux/apkrepositories` | `/etc/apk/repositories` |
| `AptRepository` | `linux/aptrepositories` | `/etc/apt/sources.list.d` |
| `DnfRepository` | `linux/dnfrepositories` | `/etc/yum.repos.d` |
| `PacmanRepository` | `linux/pacmanrepositories` | Repository sections of `/etc/pacman.conf` |

## Layout

| Path | Role |
| --- | --- |
| `resources/<name>` | One directory per kind: its handler and its tests. Fourteen of them. |
| `internal/provider` | The state every kind works from: the parsed `/etc`, the runner, the detected tooling. |
| `internal/linux` | Assembly: the provider, its documentation, and the one list of kinds. |
| `internal/pkgkind`, `internal/repokind` | The handlers the four package and four repository kinds share. |
| `internal/etcfiles` | Read-only parsers for `/etc/passwd`, `/etc/group`, `/etc/shadow`, `/etc/login.defs`. |
| `internal/usertools` | Account mutation via native tooling: shadow-utils and BusyBox. |
| `internal/pkgtools` | Package and repository backends: apk, apt, dnf/yum, pacman. |
| `internal/servicetools` | Init system abstraction: OpenRC and systemd. |
| `internal/resolvconf` | Parser and rewriter for `/etc/resolv.conf`. |
| `internal/linuxtest` | The fixture every resource package tests against. |
| `testdata` | One `/etc` that looks like a machine, read by all of them. |

## Never test mutations on this machine

This provider creates, changes and deletes **real** accounts. Every mutating
test runs inside a throwaway container — see `CLAUDE.md`.
