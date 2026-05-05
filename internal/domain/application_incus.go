package domain

func IsApplicationNameIncusKind(name string) bool {
	switch name {
	case "incus", "incus-lts-7.0":
		return true

	default:
		return false
	}
}
