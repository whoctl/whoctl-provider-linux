---
subcategory: Accounts
---

# Group

A local group. Membership has two halves, and only one of them belongs here:
`spec.members` are the supplementary members listed in `/etc/group`, while users
whose *primary* group this is are a property of the user, set through
`User.spec.primaryGroup`.

## Example

```yaml
apiVersion: linux.whoctl.io/v1alpha1
kind: Group
metadata:
  name: developers
spec:
  gid: 4000
  members: [alice, bob]
```

```console
$ whoctl get linux/groups -o wide
NAME         GID    MEMBERS     PRIMARY-MEMBERS   SYSTEM
wheel        10     alice       -                 true
developers   4000   alice,bob   carol             false
```

## Spec

<!-- whoctl:begin spec -->
| Field | Type | Notes | Description |
| --- | --- | --- | --- |
| `gid` | integer | optional | Numeric group ID. The system allocates one when omitted. Example: `4000`. |
| `members` | list of string | optional | Supplementary members listed in /etc/group. Omitting the field leaves them alone, an empty list clears them. Example: `[alice, bob]`. |
| `system` | boolean | optional, create-only | Allocate the GID outside the regular range. |
<!-- whoctl:end spec -->

## Status

<!-- whoctl:begin status -->
| Field | Type | Description |
| --- | --- | --- |
| `gid` | integer | Numeric group ID. |
| `members` | list of string | Supplementary members listed in /etc/group. |
| `primaryMembers` | list of string | Users whose primary group this is. They are not managed through spec.members and they block deletion. |
| `system` | boolean | Whether the GID falls outside the regular range declared in /etc/login.defs. |
<!-- whoctl:end status -->

## Columns

<!-- whoctl:begin columns -->
| Column | Shown |
| --- | --- |
| `NAME` | always |
| `GID` | always |
| `MEMBERS` | always |
| `PRIMARY-MEMBERS` | `-o wide` |
| `SYSTEM` | `-o wide` |
<!-- whoctl:end columns -->

## Behaviour

**Primary members block deletion.** Removing a group that is somebody's login
group would leave that account pointing at a GID with no name, so `delete`
refuses and names the users in the way.

**`spec.members` is authoritative when present.** Listing members removes
anybody not in the list; an empty list clears them; omitting the field leaves
membership alone. The same user can be added from either side — here, or through
`User.spec.groups` — but pick one per pair, or the two manifests will take turns
undoing each other.

**BusyBox has no `groupmod`.** Changing the GID of an existing group needs
shadow-utils; creating and deleting groups works on both toolsets.
