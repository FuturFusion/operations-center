package file

// UsageInformation describes the space situation of a file system.
type UsageInformation struct {
	TotalSpaceBytes     uint64
	AvailableSpaceBytes uint64
	UsedSpaceBytes      uint64
}
