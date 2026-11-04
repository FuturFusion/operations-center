package provisioning

import (
	"cmp"
	"context"
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

// ContextWithBMCRecordingScope returns a context, that carries the recording scope.
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

// ValidateBMCRecordingID rejects IDs, that are not created by BMCRecordingID.
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
// Nil means no rewrite.
type BMCRecordingRewrite struct {
	Replacements []BMCRecordingReplacement

	// GenericPatterns also replaces the MAC addresses, IPv4 addresses and UUIDs,
	// that the explicit replacements leave. The same value always gets the same
	// placeholder within one rewrite.
	GenericPatterns bool

	once         sync.Once
	pattern      *regexp.Regexp
	replacements []BMCRecordingReplacement

	genericMu sync.Mutex
	generic   bmcRecordingGenericMap
}

// BMCRecordingReplacement replaces one value in a recording.
type BMCRecordingReplacement struct {
	Old             string
	New             string
	CaseInsensitive bool

	// WholeWord skips a match, that is part of a longer word. This keeps a
	// short value like a server name from changing unrelated text.
	WholeWord bool
}

// Apply rewrites one line of text. Longer values are replaced first. Replaced
// text is not rewritten again. The generic patterns run after the explicit
// replacements.
func (r *BMCRecordingRewrite) Apply(line string) string {
	if r == nil {
		return line
	}

	// Compile once, Apply runs for every line of a recording.
	r.once.Do(r.compile)

	line = r.applyReplacements(line)

	if r.GenericPatterns {
		line = r.applyGeneric(line)
	}

	return line
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
	replacements := make([]BMCRecordingReplacement, 0, len(r.Replacements))
	for _, replacement := range r.Replacements {
		// An empty value matches everywhere.
		if replacement.Old == "" {
			continue
		}

		replacements = append(replacements, replacement)
	}

	if len(replacements) == 0 {
		return
	}

	// Go regular expressions prefer the leftmost alternative, so the longer
	// value has to come first.
	slices.SortStableFunc(replacements, func(a, b BMCRecordingReplacement) int {
		return cmp.Compare(len(b.Old), len(a.Old))
	})

	alternatives := make([]string, 0, len(replacements))
	for _, replacement := range replacements {
		alternative := "(" + regexp.QuoteMeta(replacement.Old) + ")"
		if replacement.CaseInsensitive {
			alternative = "(?i:" + alternative + ")"
		}

		alternatives = append(alternatives, alternative)
	}

	r.pattern = regexp.MustCompile(strings.Join(alternatives, "|"))
	r.replacements = replacements

	// The same value in another spelling gets the same placeholder from the
	// generic pass, and a placeholder is never replaced again.
	for _, replacement := range replacements {
		r.generic.register(replacement.Old, replacement.New)
	}
}
