package provisioning

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

const bmcRecordingIDLayout = "20060102T150405Z"

// BMCRecorder receives the HTTP exchanges with a BMC.
type BMCRecorder interface {
	// Record stores one exchange. The recorder assigns the sequence number.
	Record(ctx context.Context, exchange api.BMCRecordingExchange)

	// Stats reports the counts of the recording so far.
	Stats() BMCRecordingStats
}

// BMCRecordingStats holds the counts of a recording.
type BMCRecordingStats struct {
	Exchanges          int64
	Bytes              int64
	TruncatedExchanges int64
}

// BMCRecordingScope tells the BMC adapter to record its exchanges.
type BMCRecordingScope struct {
	Recorder BMCRecorder
	State    api.ServerDeploymentState
	Retries  int
}

type bmcRecordingScopeKey struct{}

// ContextWithBMCRecordingScope returns a context that carries the recording scope.
func ContextWithBMCRecordingScope(ctx context.Context, scope BMCRecordingScope) context.Context {
	return context.WithValue(ctx, bmcRecordingScopeKey{}, scope)
}

// BMCRecordingScopeFromContext returns the recording scope of the context, if any.
func BMCRecordingScopeFromContext(ctx context.Context) (BMCRecordingScope, bool) {
	scope, ok := ctx.Value(bmcRecordingScopeKey{}).(BMCRecordingScope)
	return scope, ok
}

// BMCRecordingID derives the recording ID from the start of the deployment.
func BMCRecordingID(startedAt time.Time) string {
	return startedAt.UTC().Format(bmcRecordingIDLayout)
}

// ValidateBMCRecordingID rejects IDs that are not created by BMCRecordingID.
// So an ID can never leave the recording directory of the server.
func ValidateBMCRecordingID(id string) error {
	ts, err := time.Parse(bmcRecordingIDLayout, id)
	if err != nil || ts.Format(bmcRecordingIDLayout) != id {
		return domain.NewValidationErrf("Invalid BMC recording ID %q", id)
	}

	return nil
}

// ServerDeploymentBMCRecording is the state of the recording kept with the deployment.
type ServerDeploymentBMCRecording struct {
	ID                 string `json:"id"`
	Exchanges          int64  `json:"exchanges"`
	Bytes              int64  `json:"bytes"`
	TruncatedExchanges int64  `json:"truncated_exchanges"`
}

// BMCRecordingRewrite replaces identifying values in a recording on export.
// Nil means no rewrite. Call Learn for every line of the recording before the
// first Apply. One rewrite serves exactly one archive.
type BMCRecordingRewrite struct {
	Replacements []BMCRecordingReplacement

	// GenericPatterns also replaces the MAC addresses, IP addresses and UUIDs
	// that the explicit replacements leave. The same value always gets the same
	// placeholder within one rewrite.
	GenericPatterns bool

	mu       sync.Mutex
	compiled bool
	pattern  *regexp.Regexp

	// replacements holds one entry per alternative of pattern.
	replacements []BMCRecordingReplacement

	learned bmcRecordingLearned
	generic bmcRecordingGenericMap
}

// BMCRecordingReplacement replaces one value in a recording.
type BMCRecordingReplacement struct {
	Old             string
	New             string
	CaseInsensitive bool

	// WholeWord skips a match that is part of a longer word. This keeps a
	// short value like a server name from changing unrelated text.
	WholeWord bool
}

// Learn collects identifying values from one line, like the value of a
// SerialNumber key at any depth. It also looks into the bodies of an exchange.
// Learn has no effect after the first Apply.
func (r *BMCRecordingRewrite) Learn(line string) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.compiled {
		return
	}

	r.learn(line)
}

// Apply rewrites one line of text. At the same position, the longer value
// wins. Replaced text is not rewritten again. The generic patterns run after
// the explicit replacements. A base64 encoded body of an exchange is decoded,
// rewritten and encoded again.
func (r *BMCRecordingRewrite) Apply(line string) string {
	if r == nil {
		return line
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.compiled {
		r.compile()
		r.compiled = true
	}

	rewritten, ok := r.applyWithBinaryBodies(line)
	if ok {
		return rewritten
	}

	return r.applyText(line)
}

func (r *BMCRecordingRewrite) applyText(text string) string {
	text = r.applyReplacements(text)

	if r.GenericPatterns {
		text = r.applyGeneric(text)
	}

	return text
}

// applyWithBinaryBodies rewrites an exchange with a base64 encoded body. The
// bodies are rewritten decoded. The text pass must not see the encoded bodies,
// since a match within them breaks the encoding. A body that cannot be decoded
// is redacted. The recorder writes the line with json.Marshal, so encoding it
// again keeps the format.
func (r *BMCRecordingRewrite) applyWithBinaryBodies(line string) (string, bool) {
	if !strings.Contains(line, `"`+api.BMCRecordingBodyEncodingBase64+`"`) {
		return "", false
	}

	var exchange api.BMCRecordingExchange

	err := json.Unmarshal([]byte(line), &exchange)
	if err != nil {
		return "", false
	}

	restoreRequest := r.takeBinaryBody(&exchange.RequestBody, &exchange.RequestBodyEncoding)
	restoreResponse := r.takeBinaryBody(&exchange.ResponseBody, &exchange.ResponseBodyEncoding)

	encoded, err := json.Marshal(exchange)
	if err != nil {
		return "", false
	}

	// Decode into a new value. Decoding into the old one merges the header
	// maps, so a header with a rewritten name keeps its original entry.
	var rewritten api.BMCRecordingExchange

	err = json.Unmarshal([]byte(r.applyText(string(encoded))), &rewritten)
	if err != nil {
		// The placeholders keep the JSON valid. Drop the exchange if not.
		rewritten = api.BMCRecordingExchange{}
	}

	restoreRequest(&rewritten.RequestBody, &rewritten.RequestBodyEncoding)
	restoreResponse(&rewritten.ResponseBody, &rewritten.ResponseBodyEncoding)

	encoded, err = json.Marshal(rewritten)
	if err != nil {
		return "", false
	}

	return string(encoded), true
}

// takeBinaryBody empties a base64 encoded body. It returns a function that
// sets the rewritten body. A body that is not base64 encoded stays.
func (r *BMCRecordingRewrite) takeBinaryBody(body *string, encoding *string) func(body *string, encoding *string) {
	if *encoding != api.BMCRecordingBodyEncodingBase64 {
		return func(*string, *string) {}
	}

	rewritten := BMCRecordingPasswordPlaceholder
	rewrittenEncoding := ""

	raw, err := base64.StdEncoding.DecodeString(*body)
	if err == nil {
		rewritten = base64.StdEncoding.EncodeToString([]byte(r.applyText(string(raw))))
		rewrittenEncoding = api.BMCRecordingBodyEncodingBase64
	}

	*body = ""
	*encoding = ""

	return func(body *string, encoding *string) {
		*body = rewritten
		*encoding = rewrittenEncoding
	}
}

func (r *BMCRecordingRewrite) applyReplacements(line string) string {
	if r.pattern == nil {
		return line
	}

	matches := r.pattern.FindAllStringSubmatchIndex(line, -1)
	if len(matches) == 0 {
		return line
	}

	var b strings.Builder
	last := 0
	for _, match := range matches {
		b.WriteString(line[last:match[0]])
		for i := range r.replacements {
			if match[2*(i+1)] < 0 {
				continue
			}

			if r.replacements[i].WholeWord && !isWholeWord(line, match[0], match[1]) {
				b.WriteString(line[match[0]:match[1]])
				break
			}

			b.WriteString(r.replacements[i].New)
			break
		}

		last = match[1]
	}

	b.WriteString(line[last:])

	return b.String()
}

func isWholeWord(line string, start int, end int) bool {
	return (start == 0 || !isWordByte(line[start-1])) && (end == len(line) || !isWordByte(line[end]))
}

func isWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

func (r *BMCRecordingRewrite) compile() {
	// The first replacement of a value wins, so an explicit value beats a
	// learned one.
	seen := map[string]bool{}
	replacements := []BMCRecordingReplacement{}
	for _, replacement := range slices.Concat(r.Replacements, r.learned.replacements) {
		// An empty value matches everywhere.
		if replacement.Old == "" || seen[replacement.Old] {
			continue
		}

		seen[replacement.Old] = true
		replacements = append(replacements, replacement)

		// The same value in another spelling gets the same placeholder from the
		// generic pass, and a placeholder is never replaced again.
		r.generic.register(replacement.Old, replacement.New)
	}

	// A value in a body is JSON escaped once in the body and once more in the
	// line of the exchange.
	spelled := []BMCRecordingReplacement{}
	seenSpelling := map[string]bool{}
	for _, replacement := range replacements {
		for _, old := range jsonEscapedSpellings(replacement.Old) {
			if seenSpelling[old] {
				continue
			}

			seenSpelling[old] = true
			replacement.Old = old
			spelled = append(spelled, replacement)
		}
	}

	if len(spelled) == 0 {
		return
	}

	// Go regular expressions prefer the leftmost alternative, so the longer
	// value has to come first.
	slices.SortStableFunc(spelled, func(a, b BMCRecordingReplacement) int {
		return cmp.Compare(len(b.Old), len(a.Old))
	})

	alternatives := make([]string, 0, len(spelled))
	for _, replacement := range spelled {
		alternative := "(" + regexp.QuoteMeta(replacement.Old) + ")"
		if replacement.CaseInsensitive {
			alternative = "(?i:" + alternative + ")"
		}

		alternatives = append(alternatives, alternative)
	}

	r.pattern = regexp.MustCompile(strings.Join(alternatives, "|"))
	r.replacements = spelled
}

// jsonEscapedSpellings returns the value as is, JSON escaped once and JSON
// escaped twice. The recorder escapes "<", ">" and "&", a BMC might not.
func jsonEscapedSpellings(value string) []string {
	plain := jsonEscape(value, false)
	html := jsonEscape(value, true)

	return []string{value, plain, html, jsonEscape(plain, true), jsonEscape(html, true)}
}

func jsonEscape(value string, escapeHTML bool) string {
	var buf bytes.Buffer

	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(escapeHTML)

	// Encoding a string does not fail.
	_ = encoder.Encode(value)

	quoted := strings.TrimSuffix(buf.String(), "\n")

	return quoted[1 : len(quoted)-1]
}
