package main

import (
	"fmt"
	"go/token"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/FuturFusion/operations-center/cmd/domain-errors/scan"
)

// finding is one place, which does not follow the conventions.
type finding struct {
	Pos  token.Position
	Rule string
	Msg  string
}

// check applies the rules to what the scan found.
func check(result *scan.Result) []finding {
	var findings []finding

	add := func(pos token.Position, rule string, format string, a ...any) {
		findings = append(findings, finding{Pos: pos, Rule: rule, Msg: fmt.Sprintf(format, a...)})
	}

	for _, wrap := range result.KindWraps {
		if wrap.Ignored {
			continue
		}

		add(wrap.Pos, "kind-without-domain-error",
			"%s classifies the error as domain.%s, which drops the reason, the hint and the details. Use domain.NewErrorf and attach the technical error with WithCause.",
			wrap.Callee, wrap.Kind)
	}

	for _, boundary := range result.BoundaryErrors {
		if boundary.Ignored {
			continue
		}

		add(boundary.Pos, "unclassified-boundary-error",
			"%s reports an error created by %s, which carries no kind, so the user only gets the generic internal server error. Classify it with domain.NewErrorf.",
			boundary.Responder, boundary.Callee)
	}

	for _, bare := range result.BareKinds {
		if bare.Ignored {
			continue
		}

		add(bare.Pos, "bare-kind-escapes",
			"domain.%s is returned as the error itself, so it reaches the user as %q without any context. Build the error with domain.NewErrorf, the kind alone is a sentinel of the repositories and adapters.",
			bare.Kind, kindText(bare.Kind))
	}

	for _, unkinded := range result.UnkindedErrors {
		if unkinded.Ignored || unkinded.Internal {
			continue
		}

		add(unkinded.Pos, "error-without-kind",
			"%s builds %q without a kind. If the user caused it, report it with domain.NewErrorf; if not, say so with a //domain-errors:internal directive.",
			unkinded.Callee, unkinded.Message)
	}

	for _, domainErr := range result.Errors {
		if domainErr.Ignored {
			continue
		}

		checkError(domainErr, add)
	}

	for _, unknown := range result.UnknownConstructs {
		add(unknown.Pos, "unknown-domain-construct",
			"domain.%s is a %s the checker does not know about, so the errors it builds pass every rule unchecked. Teach cmd/domain-errors/scan about it.",
			unknown.Name, unknown.What)
	}

	checkReasons(result, add)

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Pos.Filename != findings[j].Pos.Filename {
			return findings[i].Pos.Filename < findings[j].Pos.Filename
		}

		return findings[i].Pos.Line < findings[j].Pos.Line
	})

	return findings
}

// detailKeyRegexp mirrors the snake_case the sloglint setting of .golangci.yml
// enforces for the log, so a detail and a log attribute are named alike.
var detailKeyRegexp = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// clientSpecificHints are the ways a hint names one particular client. A hint
// is reported to every client, so it says what to do, not which command to run.
var clientSpecificHints = []string{"operations-center ", "Run ", "run the command"}

func checkError(domainErr scan.DomainError, add func(token.Position, string, string, ...any)) {
	if domainErr.MessageConst {
		checkMessage(domainErr, add)
	}

	if domainErr.HasHint && domainErr.HintConst {
		checkHint(domainErr, add)
	}

	if domainErr.ReasonUnresolved {
		add(domainErr.Pos, "reason-not-declared",
			"The reason %q is not declared in shared/api. Declare it as an api.ErrorReason constant, so a client can rely on it.", domainErr.ReasonValue)
	}

	for _, key := range domainErr.DetailKeys {
		if key == "" {
			add(domainErr.Pos, "detail-key-not-constant",
				"A WithDetail key is not a constant. Name the detail with a literal, it is part of what a client sees.")

			continue
		}

		if !detailKeyRegexp.MatchString(key) {
			add(domainErr.Pos, "detail-key-not-snake-case",
				"The WithDetail key %q is not snake_case, which is how the log names its attributes.", key)
		}
	}

	// A kind attached as the cause is found by errors.Is just like the kind of
	// the error itself, and response.SmartError matches the kinds in a fixed
	// order, so the cause silently decides the status code.
	if domainErr.CauseIsKind {
		add(domainErr.Pos, "cause-reclassifies-error",
			"WithCause attaches domain.%s, which reclassifies the error, because errors.Is finds the cause as well. Drop the cause or wrap the kind in an error of its own.",
			domainErr.CauseKind)
	}
}

func checkMessage(domainErr scan.DomainError, add func(token.Position, string, string, ...any)) {
	message := domainErr.Message

	if message == "" {
		add(domainErr.Pos, "message-empty", "The message for the user is empty.")

		return
	}

	if unicode.IsLower(firstRune(message)) {
		add(domainErr.Pos, "message-not-capitalized",
			"The message %q does not start with a capital letter.", message)
	}

	if strings.HasSuffix(message, ".") && !strings.HasSuffix(message, "..") {
		add(domainErr.Pos, "message-trailing-period",
			"The message %q ends with a period. The message is a statement of what is wrong, the hint is the sentence telling what to do.", message)
	}

	if strings.Contains(message, "%w") {
		add(domainErr.Pos, "message-wraps-error",
			"The message %q wraps an error. The message is for the user, attach the technical error with WithCause instead.", message)
	}
}

func checkHint(domainErr scan.DomainError, add func(token.Position, string, string, ...any)) {
	hint := domainErr.Hint

	if hint == "" {
		add(domainErr.Pos, "hint-empty", "The hint is empty. Leave WithHintf out, if there is nothing the user can do.")

		return
	}

	// A hint built from a value elsewhere is passed through as "%s", the
	// sentence itself is then checked at the place it is written.
	if hint == "%s" || hint == "%v" {
		return
	}

	if unicode.IsLower(firstRune(hint)) {
		add(domainErr.Pos, "hint-not-capitalized", "The hint %q does not start with a capital letter.", hint)
	}

	if !strings.HasSuffix(hint, ".") {
		add(domainErr.Pos, "hint-no-full-sentence",
			"The hint %q is not a full sentence, it does not end with a period.", hint)
	}

	for _, clientSpecific := range clientSpecificHints {
		if strings.Contains(hint, clientSpecific) {
			add(domainErr.Pos, "hint-client-specific",
				"The hint %q tells the user which command to run, but it is reported to every client. Say what to do, a client maps its own guidance from the reason.", hint)

			break
		}
	}
}

// checkReasons reports a reason, which is declared but never used. Such a
// reason is either a leftover of a condition, which is gone, or the sign of a
// condition, which was meant to be told apart and is not.
func checkReasons(result *scan.Result, add func(token.Position, string, string, ...any)) {
	for _, reason := range result.DeclaredReasons {
		if result.UsedReasons[reason.Name] {
			continue
		}

		add(reason.Pos, "reason-unused",
			"The reason %s is declared but never used. Use it or remove it, a reason, which never reaches a client, is not part of the API.", reason.Name)
	}
}

// kindText is the message a bare kind shows to the user, which is the text of
// the sentinel itself.
func kindText(kind string) string {
	text := strings.TrimPrefix(kind, "Err")

	var b strings.Builder
	for i, r := range text {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteRune(' ')
			b.WriteRune(unicode.ToLower(r))

			continue
		}

		b.WriteRune(r)
	}

	return b.String()
}

func firstRune(s string) rune {
	for _, r := range s {
		return r
	}

	return 0
}
