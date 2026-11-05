package provisioning

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/FuturFusion/operations-center/shared/api"
)

// Placeholders of an anonymized BMC recording. Host names use the reserved
// ".example" domain. IP addresses use the documentation ranges, see
// bmcRecordingIPv4Placeholder and bmcRecordingIPv6Placeholder.
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
	BMCRecordingSKUPlaceholder              = "SKU000001"
)

var (
	bmcRecordingMACPattern  = regexp.MustCompile(`^[0-9A-Fa-f]{2}([:-][0-9A-Fa-f]{2}){5}$`)
	bmcRecordingUUIDPattern = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)
	bmcRecordingIPv4Pattern = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)

	// bmcRecordingGenericPattern finds the values of the generic pass. The
	// alternatives are MAC address, UUID, IPv4 address and IPv6 address, in
	// this order. The IPv6 alternative is loose. The placeholder function checks
	// the value.
	bmcRecordingGenericPattern = regexp.MustCompile(`\b[0-9A-Fa-f]{2}(?:[:-][0-9A-Fa-f]{2}){5}\b|\b[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\b|\b(?:\d{1,3}\.){3}\d{1,3}\b|(?:[0-9A-Fa-f]{0,4}:){2,7}(?:(?:\d{1,3}\.){3}\d{1,3}|[0-9A-Fa-f]{0,4})`)

	// bmcRecordingVersionKeyPattern finds the key of a version before a value.
	// A version like 1.30.20.10 looks like an IPv4 address.
	bmcRecordingVersionKeyPattern = regexp.MustCompile(`(?i)version\\*"\s*:\s*\\*"$`)

	// bmcRecordingLearnPattern finds the string values of keys that hold
	// identifying values, in JSON text.
	bmcRecordingLearnPattern = regexp.MustCompile(`(?i)"([A-Za-z0-9_.@:-]*(?:SerialNumber|ProtectedIdentificationNumber|ServiceTag|ServiceIdentification|AssetTag|SKU|HostName|FQDN|MACAddress))"\s*:\s*"((?:[^"\\]|\\.)*)"`)
)

type bmcRecordingValueKind int

const (
	bmcRecordingValueMAC bmcRecordingValueKind = iota
	bmcRecordingValueUUID
	bmcRecordingValueIPv4
	bmcRecordingValueIPv6
)

// NewBMCRecordingRewrite collects the identifying values of a server, so an
// exported recording can be anonymized. The address of Operations Center is
// passed in, since the manifest only knows the media URLs once the recording
// is finalized.
func NewBMCRecordingRewrite(server Server, manifest api.BMCRecordingManifest, operationsCenterAddress string) *BMCRecordingRewrite {
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

	addHost := func(rawURL string, placeholder string) {
		add(urlHostname(rawURL), BMCRecordingReplacement{New: placeholder, CaseInsensitive: true, WholeWord: true})
	}

	addHost(server.BMCConfig.Endpoint, BMCRecordingBMCHostPlaceholder)
	addHost(manifest.BMCEndpoint, BMCRecordingBMCHostPlaceholder)
	add(server.BMCConfig.Username, BMCRecordingReplacement{New: BMCRecordingUsernamePlaceholder, WholeWord: true})
	add(server.BMCConfig.Password, BMCRecordingReplacement{New: BMCRecordingPasswordPlaceholder, WholeWord: true})
	addHost(server.ConnectionURL, BMCRecordingServerHostPlaceholder)
	add(server.Name, BMCRecordingReplacement{New: BMCRecordingServerPlaceholder, WholeWord: true})
	add(manifest.Server, BMCRecordingReplacement{New: BMCRecordingServerPlaceholder, WholeWord: true})
	addHost(operationsCenterAddress, BMCRecordingOperationsCenterPlaceholder)

	for _, mediaURL := range manifest.MediaURLs {
		addHost(mediaURL, BMCRecordingOperationsCenterPlaceholder)
	}

	// The token UUID is part of the media URL and grants access to the media.
	add(manifest.Request.TokenUUID, BMCRecordingReplacement{New: bmcRecordingUUIDPlaceholder(2), CaseInsensitive: true})

	macs := []string{}
	for _, data := range []api.BMCData{server.BMCData, manifest.BMCData} {
		add(data.ServerSerialNumber, BMCRecordingReplacement{New: BMCRecordingSerialNumberPlaceholder})
		add(data.ServerAssetTag, BMCRecordingReplacement{New: BMCRecordingAssetTagPlaceholder, WholeWord: true})
		add(data.BMCServiceIdentification, BMCRecordingReplacement{New: BMCRecordingServiceTagPlaceholder, WholeWord: true})
		add(data.ServerSKU, BMCRecordingReplacement{New: BMCRecordingSKUPlaceholder, WholeWord: true})
		add(data.ServerUUID, BMCRecordingReplacement{New: bmcRecordingUUIDPlaceholder(1), CaseInsensitive: true})
		add(data.ServerHostName, BMCRecordingReplacement{New: BMCRecordingServerHostPlaceholder, CaseInsensitive: true, WholeWord: true})

		for _, mac := range bmcDataMACAddresses(data) {
			key := normalizeMAC(mac)
			if !slices.Contains(macs, key) {
				macs = append(macs, key)
			}

			placeholder := bmcRecordingMACPlaceholder(slices.Index(macs, key) + 1)
			add(mac, BMCRecordingReplacement{New: placeholder, CaseInsensitive: true})

			// The generic pass misses a MAC address without separators.
			add(compactMAC(mac), BMCRecordingReplacement{New: compactMAC(placeholder), CaseInsensitive: true, WholeWord: true})
		}
	}

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

// bmcDataMACAddresses returns every string of the BMC data that is a MAC
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

func compactMAC(mac string) string {
	return strings.NewReplacer(":", "", "-", "").Replace(mac)
}

func bmcRecordingMACPlaceholder(n int) string {
	return fmt.Sprintf("02:00:00:00:%02x:%02x", n>>8&0xff, n&0xff)
}

func bmcRecordingUUIDPlaceholder(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", n)
}

// bmcRecordingIPv4Placeholder uses the documentation range 203.0.113.0/24
// (RFC 5737). More addresses continue in 198.18.0.0/15 (RFC 2544), which is
// reserved for network tests. It holds 131072 addresses.
func bmcRecordingIPv4Placeholder(n int) string {
	if n < 255 {
		return fmt.Sprintf("203.0.113.%d", n)
	}

	n -= 254

	return fmt.Sprintf("198.%d.%d.%d", 18+(n>>16&0x1), n>>8&0xff, n&0xff)
}

// bmcRecordingIPv6Placeholder uses the documentation range 2001:db8::/32
// (RFC 3849).
func bmcRecordingIPv6Placeholder(n int) string {
	return fmt.Sprintf("2001:db8::%x", n)
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

	addr, err := netip.ParseAddr(value)
	if err == nil && addr.Is6() {
		return "ipv6:" + addr.String(), bmcRecordingValueIPv6, true
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

	if kind == bmcRecordingValueIPv6 && !isIdentifyingIPv6(value) {
		return value
	}

	placeholder, ok := m.placeholders[key]
	if ok {
		return placeholder
	}

	// A value that already is a placeholder of the same kind stays.
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

		case bmcRecordingValueIPv6:
			placeholder = bmcRecordingIPv6Placeholder(n)
		}

		if !m.used[placeholder] {
			break
		}
	}

	m.used[placeholder] = true
	m.placeholders[key] = placeholder

	return placeholder
}

// isIdentifyingIPv4 reports if the address can identify a system. Masks,
// the unspecified and the loopback address do not. Keeping them keeps the
// recording readable.
func isIdentifyingIPv4(value string) bool {
	ip := net.ParseIP(value).To4()
	if ip == nil {
		return false
	}

	return !ip.IsUnspecified() && !ip.IsLoopback() && ip[0] != 255
}

// isIdentifyingIPv6 reports if the address can identify a system. A link
// local address does, since it often holds the MAC address.
func isIdentifyingIPv6(value string) bool {
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return false
	}

	return !addr.IsUnspecified() && !addr.IsLoopback() && !addr.IsMulticast()
}

func (r *BMCRecordingRewrite) applyGeneric(line string) string {
	matches := bmcRecordingGenericPattern.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return line
	}

	var b strings.Builder
	last := 0
	for _, match := range matches {
		b.WriteString(line[last:match[0]])

		value := line[match[0]:match[1]]
		if keepGenericMatch(line, match[0], match[1]) {
			b.WriteString(value)
		} else {
			b.WriteString(r.generic.placeholder(value))
		}

		last = match[1]
	}

	b.WriteString(line[last:])

	return b.String()
}

// keepGenericMatch reports if a match of the generic pass is no identifying
// value. This is a version with four parts, or text like "Foo::Bar" that only
// looks like an IPv6 address.
func keepGenericMatch(line string, start int, end int) bool {
	value := line[start:end]

	if bmcRecordingIPv4Pattern.MatchString(value) {
		return bmcRecordingVersionKeyPattern.MatchString(line[max(0, start-64):start])
	}

	if strings.Contains(value, ":") && !bmcRecordingMACPattern.MatchString(value) {
		return !isIPv6Boundary(line, start-1) || !isIPv6Boundary(line, end)
	}

	return false
}

func isIPv6Boundary(line string, i int) bool {
	if i < 0 || i >= len(line) {
		return true
	}

	return !isWordByte(line[i]) && line[i] != ':' && line[i] != '.'
}

// bmcRecordingLearnCategory maps the end of a key to the placeholder of its
// value.
type bmcRecordingLearnCategory struct {
	suffix          string
	format          string
	caseInsensitive bool
}

var bmcRecordingLearnCategories = []bmcRecordingLearnCategory{
	{suffix: "serialnumber", format: "SERIAL%06d"},
	{suffix: "protectedidentificationnumber", format: "SERIAL%06d"},
	{suffix: "servicetag", format: "SERVICE%06d"},
	{suffix: "serviceidentification", format: "SERVICE%06d"},
	{suffix: "assettag", format: "ASSET%06d"},
	{suffix: "sku", format: "SKU%06d"},
	{suffix: "hostname", format: "host%d.example", caseInsensitive: true},
	{suffix: "fqdn", format: "host%d.example", caseInsensitive: true},
}

// bmcRecordingFillerValues are values of identifying keys that identify
// nothing.
var bmcRecordingFillerValues = []string{"default", "localhost", "none", "null", "unknown", "unspecified"}

// bmcRecordingLearned holds the values that Learn has found.
type bmcRecordingLearned struct {
	replacements []BMCRecordingReplacement
	known        map[string]bool
	placeholders map[string]bool
	next         map[string]int
}

func (r *BMCRecordingRewrite) learn(line string) {
	r.learnText(line)

	var exchange api.BMCRecordingExchange

	err := json.Unmarshal([]byte(line), &exchange)
	if err != nil {
		return
	}

	r.learnBody(exchange.RequestBody, exchange.RequestBodyEncoding)
	r.learnBody(exchange.ResponseBody, exchange.ResponseBodyEncoding)
}

func (r *BMCRecordingRewrite) learnBody(body string, encoding string) {
	if encoding == api.BMCRecordingBodyEncodingBase64 {
		raw, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			return
		}

		body = string(raw)
	}

	r.learnText(body)
}

// learnText uses a regular expression and no JSON parser. So it also finds
// the values in a truncated body.
func (r *BMCRecordingRewrite) learnText(text string) {
	for _, match := range bmcRecordingLearnPattern.FindAllStringSubmatch(text, -1) {
		var value string

		err := json.Unmarshal([]byte(`"`+match[2]+`"`), &value)
		if err != nil {
			continue
		}

		r.learnValue(strings.ToLower(match[1]), value)
	}
}

func (r *BMCRecordingRewrite) learnValue(key string, value string) {
	l := &r.learned
	if l.known == nil {
		l.known = map[string]bool{}
		l.placeholders = map[string]bool{}
		l.next = map[string]int{}

		// A learned MAC address gets its placeholder from the generic pass, so
		// the explicit values have to be known there first.
		for _, replacement := range r.Replacements {
			l.known[replacement.Old] = true
			l.placeholders[replacement.New] = true
			r.generic.register(replacement.Old, replacement.New)
		}
	}

	if l.known[value] || l.placeholders[value] {
		return
	}

	if strings.HasSuffix(key, "macaddress") {
		r.learnMAC(value)
		return
	}

	for _, category := range bmcRecordingLearnCategories {
		if !strings.HasSuffix(key, category.suffix) || !isLearnableValue(value) {
			continue
		}

		placeholder := ""
		for placeholder == "" || l.placeholders[placeholder] {
			l.next[category.format]++
			placeholder = fmt.Sprintf(category.format, l.next[category.format])
		}

		l.known[value] = true
		l.placeholders[placeholder] = true
		l.replacements = append(l.replacements, BMCRecordingReplacement{
			Old:             value,
			New:             placeholder,
			CaseInsensitive: category.caseInsensitive,
			WholeWord:       true,
		})

		return
	}
}

// learnMAC adds a MAC address without separators. The generic pass handles
// the other spellings.
func (r *BMCRecordingRewrite) learnMAC(value string) {
	compact := strings.ToLower(compactMAC(value))
	if r.learned.known[compact] || len(compact) != 12 || strings.Trim(compact, "0") == "" || strings.Trim(compact, "f") == "" {
		return
	}

	colon := compact[0:2] + ":" + compact[2:4] + ":" + compact[4:6] + ":" + compact[6:8] + ":" + compact[8:10] + ":" + compact[10:12]
	if !bmcRecordingMACPattern.MatchString(colon) {
		return
	}

	r.learned.known[value] = true
	r.learned.known[compact] = true
	r.learned.replacements = append(r.learned.replacements, BMCRecordingReplacement{
		Old:             compact,
		New:             compactMAC(r.generic.placeholder(colon)),
		CaseInsensitive: true,
		WholeWord:       true,
	})
}

// isLearnableValue skips values that are too short or too common to be
// replaced everywhere in the recording. Such a value would change unrelated
// text.
func isLearnableValue(value string) bool {
	if len(value) < 4 || strings.ContainsFunc(value, unicode.IsSpace) {
		return false
	}

	if slices.Contains(bmcRecordingFillerValues, strings.ToLower(value)) {
		return false
	}

	// A short number like 1000 is also a speed or a size.
	if len(value) < 8 && strings.Trim(value, "0123456789") == "" {
		return false
	}

	// A value of one repeated character, like 0000000000 or "........".
	return strings.Trim(value, value[:1]) != ""
}
