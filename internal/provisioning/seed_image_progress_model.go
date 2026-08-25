package provisioning

import (
	"net"
	"net/netip"
	"time"
)

// SeedImageID addresses one prepared seed image, the way the URL a BMC streams
// it from does.
type SeedImageID struct {
	CacheID       string
	FingerprintID string
}

func (i SeedImageID) String() string {
	return i.CacheID + "/" + i.FingerprintID
}

// SeedImageProgress reports how much of a seed image one source has read.
type SeedImageProgress struct {
	ImageID       SeedImageID
	Source        string
	Size          int64
	BytesServed   int64
	HighestOffset int64
	FirstRead     time.Time
	LastRead      time.Time
	RequestCount  int
}

// PercentComplete returns how much of the image has been read, based on the
// highest offset reached. It returns 0, if the size of the image is unknown.
func (p SeedImageProgress) PercentComplete() float64 {
	if p.Size <= 0 {
		return 0
	}

	return min(float64(p.HighestOffset)/float64(p.Size)*100, 100)
}

// IdleFor returns for how long nothing has been read anymore. It returns 0, if
// nothing has been read at all.
func (p SeedImageProgress) IdleFor(now time.Time) time.Duration {
	if p.LastRead.IsZero() {
		return 0
	}

	return now.Sub(p.LastRead)
}

// SeedImageSource returns the canonical form of the address an image is read
// from, as it is used to tell the readers of an image apart.
func SeedImageSource(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	// Normalize the host, so that an address recorded here can be compared to
	// the host of a configured BMC endpoint.
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}

	return ip.Unmap().String()
}
