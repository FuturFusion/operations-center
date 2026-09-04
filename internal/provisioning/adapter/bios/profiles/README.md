# BIOS Profiles

## Deferred attributes

`attributes` are applied in one go and picked up by the firmware on the next
reset. An attribute, that the firmware only accepts once another attribute is
actually in effect belongs into `deferred_attributes`, which the automated
deployment applies in a second pass, after the server has been rebooted and the
attributes of the first pass are in effect.

## Secure boot allow lists

The automated deployment wipes the `KEK`, `db` and `dbx` key databases of a
server and reinitializes them with the certificates of IncusOS. The `secure_boot`
section of a profile names the entries, that survive that wipe, keyed by the
lower case hex encoded SHA256 fingerprint of the certificate or by the signature
value:

* `true` keeps the entry.
* `false` removes it, which is how one of the entries allow listed by Operations
  Center itself is dropped again. Out of the box those are the Microsoft CAs in
  `db`, which the option ROMs of most hardware are signed with.
* A null value drops the entry from the set accumulated by the profiles resolved
  before, so a profile with a higher priority can undo a decision rather than
  overrule it.

A key database, that holds the certificates of IncusOS plus the allow listed
entries and nothing else, is left untouched entirely, which spares a server, that
is deployed a second time, having its UEFI keys deleted and rewritten.

## Example

An example of a BIOS profile, showing all the fields available for matching and
for the values to apply, can be found in
[`_dummy.yaml.example`](_dummy.yaml.example).

## Tests

If new profiles are added, it is advised to also update
[`../profiles_test.go`](../profiles_test.go) with additional tests for the
manufacturers, models and hardware combinations covered by the new profiles, to
make sure the profiles provide the expected outcome.
