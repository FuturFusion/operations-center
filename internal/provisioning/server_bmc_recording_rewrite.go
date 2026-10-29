package provisioning

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/FuturFusion/operations-center/shared/api"
)

// Placeholders of an anonymized BMC recording. They use reserved names and the
// documentation ranges, so they never identify a real system.
const (
	BMCRecordingServerPlaceholder           = "server01"
	BMCRecordingBMCHostPlaceholder          = "bmc.example"
	BMCRecordingServerHostPlaceholder       = "server.example"
	BMCRecordingOperationsCenterPlaceholder = "operations-center.example"
	BMCRecordingUsernamePlaceholder         = "bmc-user"
	BMCRecordingPasswordPlaceholder         = "<redacted>"
	BMCRecordingSerialNumberPlaceholder     = "SERIAL000001"
	BMCRecordingAssetTagPlaceholder         = "ASSET000001"
	BMCRecordingServiceTagPlaceholder       = "SERVICE000001"
)

var (
	bmcRecordingMACPattern  = regexp.MustCompile(`^[0-9A-Fa-f]{2}([:-][0-9A-Fa-f]{2}){5}$`)
	bmcRecordingUUIDPattern = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)
	bmcRecordingIPv4Pattern = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)

	// bmcRecordingGenericPattern finds the values of the generic pass. The
	// groups are MAC address, UUID and IPv4 address, in this order.
	bmcRecordingGenericPattern = regexp.MustCompile(`\b([0-9A-Fa-f]{2}(?:[:-][0-9A-Fa-f]{2}){5})\b|\b([0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12})\b|\b((?:\d{1,3}\.){3}\d{1,3})\b`)
)

type bmcRecordingValueKind int

const (
	bmcRecordingValueMAC bmcRecordingValueKind = iota
	bmcRecordingValueUUID
	bmcRecordingValueIPv4
)

// NewBMCRecordingRewrite collects the identifying values of a server, so an
// exported recording can be published.
func NewBMCRecordingRewrite(server Server, manifest api.BMCRecordingManifest) *BMCRecordingRewrite {
	replacements := []BMCRecordingReplacement{}
	seen := map[string]bool{}

	// The first replacement of a value wins.
	add := func(old string, replacement BMCRecordingReplacement) {
		if old == "" || seen[old] {
			return
		}

		seen[old] = true
		replacement.Old = old
		replacements = append(replacements, replacement)
	}

	add(urlHostname(server.BMCConfig.Endpoint), BMCRecordingReplacement{New: BMCRecordingBMCHostPlaceholder, WholeWord: true})
	add(urlHostname(manifest.BMCEndpoint), BMCRecordingReplacement{New: BMCRecordingBMCHostPlaceholder, WholeWord: true})
	add(server.BMCConfig.Username, BMCRecordingReplacement{New: BMCRecordingUsernamePlaceholder, WholeWord: true})
	add(server.BMCConfig.Password, BMCRecordingReplacement{New: BMCRecordingPasswordPlaceholder, WholeWord: true})
	add(urlHostname(server.ConnectionURL), BMCRecordingReplacement{New: BMCRecordingServerHostPlaceholder, WholeWord: true})
	add(server.Name, BMCRecordingReplacement{New: BMCRecordingServerPlaceholder, WholeWord: true})
	add(manifest.Server, BMCRecordingReplacement{New: BMCRecordingServerPlaceholder, WholeWord: true})

	for _, mediaURL := range manifest.MediaURLs {
		add(urlHostname(mediaURL), BMCRecordingReplacement{New: BMCRecordingOperationsCenterPlaceholder, WholeWord: true})
	}

	macs := []string{}
	for _, data := range []api.BMCData{server.BMCData, manifest.BMCData} {
		add(data.ServerSerialNumber, BMCRecordingReplacement{New: BMCRecordingSerialNumberPlaceholder})
		add(data.ServerAssetTag, BMCRecordingReplacement{New: BMCRecordingAssetTagPlaceholder, WholeWord: true})
		add(data.BMCServiceIdentification, BMCRecordingReplacement{New: BMCRecordingServiceTagPlaceholder, WholeWord: true})
		add(data.ServerUUID, BMCRecordingReplacement{New: bmcRecordingUUIDPlaceholder(1), CaseInsensitive: true})
		add(data.ServerHostName, BMCRecordingReplacement{New: BMCRecordingServerHostPlaceholder, WholeWord: true})

		for _, mac := range bmcDataMACAddresses(data) {
			key := normalizeMAC(mac)
			if !slices.Contains(macs, key) {
				macs = append(macs, key)
			}

			add(mac, BMCRecordingReplacement{New: bmcRecordingMACPlaceholder(slices.Index(macs, key) + 1), CaseInsensitive: true})
		}
	}

	slices.SortStableFunc(replacements, func(a, b BMCRecordingReplacement) int {
		return cmp.Compare(len(b.Old), len(a.Old))
	})

	return &BMCRecordingRewrite{
		Replacements:    replacements,
		GenericPatterns: true,
	}
}

func urlHostname(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	return u.Hostname()
}

// bmcDataMACAddresses returns every string of the BMC data, that is a MAC
// address. The JSON walk finds them without a list of the fields.
func bmcDataMACAddresses(data api.BMCData) []string {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil
	}

	var value any

	err = json.Unmarshal(raw, &value)
	if err != nil {
		return nil
	}

	var macs []string

	var walk func(value any)
	walk = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for _, key := range slices.Sorted(maps.Keys(value)) {
				walk(value[key])
			}

		case []any:
			for _, child := range value {
				walk(child)
			}

		case string:
			if bmcRecordingMACPattern.MatchString(value) && !slices.Contains(macs, value) {
				macs = append(macs, value)
			}
		}
	}

	walk(value)

	return macs
}

func normalizeMAC(mac string) string {
	return strings.ToLower(strings.ReplaceAll(mac, "-", ":"))
}

func bmcRecordingMACPlaceholder(n int) string {
	return fmt.Sprintf("02:00:00:00:%02x:%02x", n>>8&0xff, n&0xff)
}

func bmcRecordingUUIDPlaceholder(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", n)
}

func bmcRecordingIPv4Placeholder(n int) string {
	if n < 255 {
		return fmt.Sprintf("203.0.113.%d", n)
	}

	// The benchmark range is large enough for any recording.
	n -= 254

	return fmt.Sprintf("198.18.%d.%d", n>>8&0xff, n&0xff)
}

// bmcRecordingGenericMap holds the placeholders of the generic pass. It lives
// on one rewrite, so the placeholders are stable within one archive only.
type bmcRecordingGenericMap struct {
	placeholders map[string]string
	used         map[string]bool
	next         map[bmcRecordingValueKind]int
}

func (m *bmcRecordingGenericMap) init() {
	if m.placeholders != nil {
		return
	}

	m.placeholders = map[string]string{}
	m.used = map[string]bool{}
	m.next = map[bmcRecordingValueKind]int{}
}

func bmcRecordingValueKey(value string) (string, bmcRecordingValueKind, bool) {
	switch {
	case bmcRecordingMACPattern.MatchString(value):
		return "mac:" + normalizeMAC(value), bmcRecordingValueMAC, true

	case bmcRecordingUUIDPattern.MatchString(value):
		return "uuid:" + strings.ToLower(value), bmcRecordingValueUUID, true

	case bmcRecordingIPv4Pattern.MatchString(value):
		return "ipv4:" + value, bmcRecordingValueIPv4, true
	}

	return "", 0, false
}

// register maps an explicit value to its placeholder and keeps the
// placeholder itself from being replaced.
func (m *bmcRecordingGenericMap) register(old string, placeholder string) {
	m.init()

	m.used[placeholder] = true

	key, _, ok := bmcRecordingValueKey(old)
	if ok {
		m.placeholders[key] = placeholder
	}

	key, _, ok = bmcRecordingValueKey(placeholder)
	if ok {
		m.placeholders[key] = placeholder
	}
}

func (m *bmcRecordingGenericMap) placeholder(value string) string {
	m.init()

	key, kind, ok := bmcRecordingValueKey(value)
	if !ok {
		return value
	}

	if kind == bmcRecordingValueIPv4 && !isIdentifyingIPv4(value) {
		return value
	}

	placeholder, ok := m.placeholders[key]
	if ok {
		return placeholder
	}

	// A value, that already is a placeholder of the same kind, stays.
	if m.used[value] {
		return value
	}

	for {
		m.next[kind]++
		n := m.next[kind]

		switch kind {
		case bmcRecordingValueMAC:
			placeholder = bmcRecordingMACPlaceholder(n)

		case bmcRecordingValueUUID:
			placeholder = bmcRecordingUUIDPlaceholder(n)

		case bmcRecordingValueIPv4:
			placeholder = bmcRecordingIPv4Placeholder(n)
		}

		if !m.used[placeholder] {
			break
		}
	}

	m.used[placeholder] = true
	m.placeholders[key] = placeholder

	return placeholder
}

// isIdentifyingIPv4 reports, if the address can identify a system. Masks,
// the unspecified and the loopback address do not, and keeping them keeps the
// recording readable.
func isIdentifyingIPv4(value string) bool {
	ip := net.ParseIP(value).To4()
	if ip == nil {
		return false
	}

	return !ip.IsUnspecified() && !ip.IsLoopback() && ip[0] != 255
}

func (r *BMCRecordingRewrite) applyGeneric(line string) string {
	r.genericMu.Lock()
	defer r.genericMu.Unlock()

	return bmcRecordingGenericPattern.ReplaceAllStringFunc(line, r.generic.placeholder)
}
