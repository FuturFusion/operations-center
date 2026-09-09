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
* `false` removes it again, which only ever undoes a `true` of a profile resolved
  before, since nothing is allow listed out of the box.
* A null value drops the entry from the set accumulated by the profiles resolved
  before, so a profile with a higher priority can undo a decision rather than
  overrule it.

The profiles are the only source of the allow list, so a profile, that names
nothing, has the `db` of its servers wiped down to the certificates of IncusOS
and the option ROMs of the hardware stop being trusted. A profile for hardware,
that none of the profiles shipped with Operations Center match, therefore has to
name the Microsoft CAs itself, see [`_dummy.yaml.example`](_dummy.yaml.example).

A key database, that holds the certificates of IncusOS plus the allow listed
entries and nothing else, is left untouched entirely, which spares a server, that
is deployed a second time, having its UEFI keys deleted and rewritten.

The allow list also decides, what the secure boot enrollment media re-enrolls. A
deployment, that enrolls the certificates from an enrollment media rather than
through the Redfish API, starts from wiped key databases, so an allow listed
entry has to be enrolled again from actual certificate material. That material
comes from the certificate catalog in
[`../../securebootcerts`](../../securebootcerts). An entry, that the catalog
does not know, is reported as a warning and lost. The same holds for an allow
listed signature: the enrollment media enrolls certificate material only, so a
signature can not be enrolled again.

## Example

An example of a BIOS profile, showing all the fields available for matching and
for the values to apply, can be found in
[`_dummy.yaml.example`](_dummy.yaml.example).

## Tests

If new profiles are added, it is advised to also update
[`../profiles_test.go`](../profiles_test.go) with additional tests for the
manufacturers, models and hardware combinations covered by the new profiles, to
make sure the profiles provide the expected outcome.
