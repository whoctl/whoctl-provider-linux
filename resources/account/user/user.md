---
subcategory: Accounts
---

# User

A local login account. `spec` is the account as you want it; anything omitted is
left to the native tool's default when the account is created, and left untouched
when it already exists.

## Example

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
  home: /home/alice
  comment: Alice Liddell
  locked: false
```

```console
$ whoctl apply -f alice.yaml
linux/user/alice created

$ whoctl get linux/users
NAME     UID     GID     GROUP        SHELL
root     0       0       root         /bin/sh
alice    4200    4000    developers   /bin/sh
nobody   65534   65534   nobody       /sbin/nologin
```

## Spec

<!-- whoctl:begin spec -->
| Field | Type | Notes | Description |
| --- | --- | --- | --- |
| `uid` | integer | optional | Numeric user ID. The system allocates one when omitted. Example: `4200`. |
| `primaryGroup` | string | optional | Login group of the account. It must already exist. Example: `developers`. |
| `groups` | list of string | optional | Supplementary groups. Omitting the field leaves memberships alone, an empty list removes every one of them. Example: `[wheel]`. |
| `shell` | string | optional | Login shell. Example: `/bin/sh`. |
| `home` | string | optional | Home directory. Changing it on an existing account moves the contents across. Example: `/home/alice`. |
| `comment` | string | optional | The GECOS field, usually the full name of the person. Example: `Alice Liddell`. |
| `system` | boolean | optional, create-only | Allocate the UID outside the regular range, for accounts owned by a package rather than a person. |
| `createHome` | boolean | optional, create-only | Whether to create the home directory. Left to the native tool when omitted. |
| `locked` | boolean | optional | Whether the account is barred from logging in. An account created without a password comes out locked. |
| `passwordHash` | string | optional, write-only | A crypt(3) hash, never a plaintext password. Example: `$6$rounds=...`. |
<!-- whoctl:end spec -->

## Status

<!-- whoctl:begin status -->
| Field | Type | Description |
| --- | --- | --- |
| `uid` | integer | Numeric user ID. |
| `gid` | integer | Numeric ID of the primary group. |
| `primaryGroup` | string | Name of the primary group, resolved from the GID. |
| `groups` | list of string | Supplementary groups the account belongs to. |
| `home` | string | Home directory recorded in /etc/passwd. |
| `homeExists` | boolean | Whether that directory is actually on disk. |
| `shell` | string | Login shell. |
| `system` | boolean | Whether the UID falls outside the regular range declared in /etc/login.defs. |
| `locked` | boolean | Whether the password field in /etc/shadow is locked. Reads as false when /etc/shadow cannot be read. |
| `passwordSet` | boolean | Whether the account has a usable password. |
<!-- whoctl:end status -->

## Columns

<!-- whoctl:begin columns -->
| Column | Shown |
| --- | --- |
| `NAME` | always |
| `UID` | always |
| `GID` | always |
| `GROUP` | always |
| `SHELL` | always |
| `HOME` | `-o wide` |
| `GROUPS` | `-o wide` |
| `LOCKED` | `-o wide` |
| `SYSTEM` | `-o wide` |
<!-- whoctl:end columns -->

## Behaviour

**A new account comes out locked.** Creating one without a password leaves no
usable password behind, which is what the native tools do, and whoctl reports it
in `status.locked` rather than hiding it. Set `spec.passwordHash`, or
`locked: false` once a password exists, to change that.

**The password hash is never read back.** `get` leaves `spec.passwordHash` out
of its output, so an exported manifest cannot leak a credential — and `apply`
stays idempotent because the hash is compared, not returned.

**Supplementary groups are three-state.** Omitting `groups` leaves memberships
alone. Listing them makes the list authoritative: whoever is not in it is
removed. An empty list removes them all. The primary group is not one of them —
it is `spec.primaryGroup`, and it must already exist.

**Moving the home directory moves its contents.** Changing `spec.home` on an
existing account passes the move on to the native tool rather than leaving the
old directory behind.

**BusyBox cannot modify an existing account.** On a system with no shadow-utils,
creating and deleting accounts works, and reconciling one fails with the
`apk add shadow` hint instead of silently doing nothing.

Deleting a user leaves the home directory in place unless `--cascade` is given.
