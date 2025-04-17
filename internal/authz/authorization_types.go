package authz

// Entitlement is a type representation of a permission as it applies to a particular ObjectType.
type Entitlement string

const (
	// Entitlements that apply to all resources.
	EntitlementCanCreate Entitlement = "can_create"
	EntitlementCanDelete Entitlement = "can_delete"
	EntitlementCanEdit   Entitlement = "can_edit"
	EntitlementCanView   Entitlement = "can_view"
)

// String implements fmt.Stringer for Entitlement.
func (e Entitlement) String() string {
	return string(e)
}

// ObjectType is a type of resource within the operations center.
type ObjectType string

const (
	// ObjectTypeUser represents a user.
	ObjectTypeUser ObjectType = "user"

	// ObjectTypeServer represents a server.
	ObjectTypeServer ObjectType = "server"
)

// String implements fmt.Stringer for ObjectType.
func (o ObjectType) String() string {
	return string(o)
}
