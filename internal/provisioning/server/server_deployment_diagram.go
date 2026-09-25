package server

import (
	"fmt"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

// deploymentDiagramEntryState is, where a deployment starts. The cancel branch
// and the terminal states are entered from anywhere, so they are laid out after
// the happy path rather than being walked into.
const deploymentDiagramEntryState = api.ServerDeploymentStateRefreshBMCData

// deploymentDiagramSkipFlags are the flags, the pass by decisions are taken on.
// The diagram draws an edge per outcome, so every combination of them is walked.
func deploymentDiagramSkipFlags() []*provisioning.ServerDeployment {
	deployments := make([]*provisioning.ServerDeployment, 0, 256)

	for flags := range 256 {
		deployment := &provisioning.ServerDeployment{
			BIOSPending:            flags&1 != 0,
			BIOSDeferredPending:    flags&2 != 0,
			SecureBootPending:      flags&4 != 0,
			SecureBootResetPending: flags&16 != 0,
			Request: provisioning.ServerDeploymentRequest{
				SkipSecureBootCertificates: flags&8 != 0,
				SecureBootEnrollmentMedia:  flags&32 != 0,
			},
		}

		if flags&64 != 0 {
			deployment.SecureBootResetTaskMonitor = "task"
		}

		if flags&128 != 0 {
			deployment.BIOSSecureBootPendingAttributes = []string{"attribute"}
		}

		deployments = append(deployments, deployment)
	}

	return deployments
}

// deploymentDiagramDuration renders a duration the way the documentation reads
// it, so an hour is "1h" rather than "1h0m0s".
func deploymentDiagramDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}

	out := &strings.Builder{}

	for _, unit := range []struct {
		size   time.Duration
		suffix string
	}{
		{size: time.Hour, suffix: "h"},
		{size: time.Minute, suffix: "m"},
		{size: time.Second, suffix: "s"},
	} {
		count := d / unit.size
		if count == 0 {
			continue
		}

		fmt.Fprintf(out, "%d%s", count, unit.suffix)

		d -= count * unit.size
	}

	return out.String()
}

// deploymentDiagramConditionView is, what a condition template is rendered
// against. It exposes the durations of the state, so a condition never spells a
// duration out a second time.
type deploymentDiagramConditionView struct {
	SettleDelay         string
	PowerOffSettleDelay string
	RebootWindow        string
	Timeout             string
}

// deploymentDiagramCondition renders the condition of a state.
func deploymentDiagramCondition(definition deploymentStateDefinition) (string, error) {
	if definition.condition == "" {
		return "", nil
	}

	tmpl, err := template.New("condition").Option("missingkey=error").Parse(definition.condition)
	if err != nil {
		return "", fmt.Errorf("Failed to parse the condition %q: %w", definition.condition, err)
	}

	view := deploymentDiagramConditionView{
		SettleDelay:         deploymentDiagramDuration(definition.settleDelay),
		PowerOffSettleDelay: deploymentDiagramDuration(definition.powerOffSettleDelay),
		RebootWindow:        deploymentDiagramDuration(definition.rebootWindow),
		Timeout:             deploymentDiagramDuration(definition.timeout),
	}

	out := &strings.Builder{}

	err = tmpl.Execute(out, view)
	if err != nil {
		return "", fmt.Errorf("Failed to render the condition %q: %w", definition.condition, err)
	}

	return out.String(), nil
}

// deploymentDiagramEdge is a single transition of the rendered diagram.
type deploymentDiagramEdge struct {
	from  string
	to    string
	label string
}

// deploymentDiagramTarget resolves, where a transition to next actually lands
// for the given deployment, and collects, why. A state, that is passed by,
// contributes its skip reason, or its branch reason, where it routes to one of
// its branches, the state, that is finally entered, its enter reason.
func deploymentDiagramTarget(deployment *provisioning.ServerDeployment, next api.ServerDeploymentState) (api.ServerDeploymentState, []string) {
	var reasons []string

	for range len(deploymentStates) {
		definition := deploymentStates[next]
		if definition.enterState == nil {
			return next, reasons
		}

		entered := definition.enterState(deployment)
		if entered == next {
			if definition.enterReason != "" {
				reasons = append(reasons, definition.enterReason)
			}

			return next, reasons
		}

		reason := definition.skipReason
		if slices.Contains(definition.branches, entered) {
			reason = definition.branchReason
		}

		if reason != "" {
			reasons = append(reasons, reason)
		}

		next = entered
	}

	return next, reasons
}

// deploymentDiagramOrder lays the states out along the happy path, followed by
// the branches off it and the states only a revert leads to, so the rendered diagram does not churn with the iteration
// order of the table.
func deploymentDiagramOrder() []api.ServerDeploymentState {
	var (
		order    []api.ServerDeploymentState
		branches []api.ServerDeploymentState
	)

	seen := map[api.ServerDeploymentState]struct{}{}

	walk := func(state api.ServerDeploymentState) {
		for state != "" && !state.IsTerminal() {
			_, ok := seen[state]
			if ok {
				return
			}

			seen[state] = struct{}{}
			order = append(order, state)
			branches = append(branches, deploymentStates[state].branches...)

			if deploymentStates[state].revert != "" {
				branches = append(branches, deploymentStates[state].revert)
			}

			state = deploymentStates[state].next
		}
	}

	walk(deploymentDiagramEntryState)

	for len(branches) > 0 {
		branch := branches[0]
		branches = branches[1:]

		walk(branch)
	}

	walk(api.ServerDeploymentStateCancel)

	return append(order, api.ServerDeploymentTerminalStates()...)
}

// deploymentDiagramEdges collects every transition the diagram draws, in the
// order the states are laid out.
//
// The edges into failed on an exhausted retry budget and into cancel on a cancel
// request are deliberately left out: every state has both, and drawing one of
// each per state would bury the machine. The prose next to the diagram says so.
func deploymentDiagramEdges(order []api.ServerDeploymentState) ([]deploymentDiagramEdge, error) {
	rank := map[api.ServerDeploymentState]int{}
	for i, state := range order {
		rank[state] = i
	}

	edges := []deploymentDiagramEdge{{
		from:  "[*]",
		to:    string(deploymentDiagramEntryState),
		label: "deploy triggered",
	}}

	for _, state := range order {
		definition := deploymentStates[state]

		if definition.kind == deploymentStateKindTerminal {
			edges = append(edges, deploymentDiagramEdge{from: string(state), to: "[*]"})
			continue
		}

		condition, err := deploymentDiagramCondition(definition)
		if err != nil {
			return nil, fmt.Errorf("Failed to render the condition of state %q: %w", state, err)
		}

		// A transition to a state, that can be passed by, lands somewhere else
		// depending on the deployment, so every outcome gets an edge of its own.
		targets := map[api.ServerDeploymentState]string{}

		var targetOrder []api.ServerDeploymentState

		for _, deployment := range deploymentDiagramSkipFlags() {
			target, reasons := deploymentDiagramTarget(deployment, definition.next)

			_, ok := targets[target]
			if ok {
				continue
			}

			label := strings.Join(append([]string{condition}, reasons...), ", ")
			targets[target] = strings.Trim(label, ", ")

			targetOrder = append(targetOrder, target)
		}

		slices.SortFunc(targetOrder, func(a api.ServerDeploymentState, b api.ServerDeploymentState) int {
			return rank[a] - rank[b]
		})

		for _, target := range targetOrder {
			edges = append(edges, deploymentDiagramEdge{from: string(state), to: string(target), label: targets[target]})
		}

		// A verification, that finds the attributes unapplied, repeats the pass,
		// that applied them, rather than being retried in place.
		if definition.retryFrom != "" {
			edges = append(edges, deploymentDiagramEdge{from: string(state), to: string(definition.retryFrom), label: definition.retryReason})
		}

		if definition.kind != deploymentStateKindWait {
			continue
		}

		// A wait, that observes something contradicting the step before it,
		// sends the deployment back to it.
		if definition.revert != "" {
			edges = append(edges, deploymentDiagramEdge{from: string(state), to: string(definition.revert), label: definition.revertReason})
		}

		timeout := "timeout (" + deploymentDiagramDuration(definition.timeout) + ")"

		// A wait, that has a trigger to fall back to, re-issues it. One, that has
		// none, ends the deployment instead.
		if definition.fallback != "" {
			edges = append(edges, deploymentDiagramEdge{from: string(state), to: string(definition.fallback), label: timeout})
			continue
		}

		edges = append(edges, deploymentDiagramEdge{from: string(state), to: string(api.ServerDeploymentStateFailed), label: timeout})
	}

	return edges, nil
}

// deploymentDiagramAcronyms keep their spelling in an identifier, which a plain
// title case of the kebab cased state name would flatten to "Bios".
var deploymentDiagramAcronyms = map[string]string{
	"bios": "BIOS",
	"bmc":  "BMC",
	"os":   "OS",
}

// deploymentDiagramID is the identifier of a state in the diagram. The state
// names are kebab case, which mermaid does not accept unquoted.
func deploymentDiagramID(state api.ServerDeploymentState) string {
	out := &strings.Builder{}

	for part := range strings.SplitSeq(string(state), "-") {
		if part == "" {
			continue
		}

		acronym, ok := deploymentDiagramAcronyms[part]
		if ok {
			out.WriteString(acronym)
			continue
		}

		out.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}

	return out.String()
}

// DeploymentStateDiagram renders the deployment state machine as a mermaid state
// diagram. It is the source of the diagram in the documentation and is exported
// for the generator, which can not read the table itself.
func DeploymentStateDiagram() (string, error) {
	order := deploymentDiagramOrder()

	for state := range deploymentStates {
		if !slices.Contains(order, state) {
			//domain-errors:internal Programmer error, the state table is inconsistent.
			return "", fmt.Errorf("State %q can not be reached from the entry states, so the diagram would leave it out", state)
		}
	}

	edges, err := deploymentDiagramEdges(order)
	if err != nil {
		return "", err
	}

	out := &strings.Builder{}

	out.WriteString("```{mermaid}\n:zoom:\nstateDiagram-v2\n")

	for _, state := range order {
		fmt.Fprintf(out, "    state %q as %s\n", deploymentStates[state].label, deploymentDiagramID(state))
	}

	out.WriteString("\n")

	for _, edge := range edges {
		from := edge.from
		if from != "[*]" {
			from = deploymentDiagramID(api.ServerDeploymentState(from))
		}

		to := edge.to
		if to != "[*]" {
			to = deploymentDiagramID(api.ServerDeploymentState(to))
		}

		if edge.label == "" {
			fmt.Fprintf(out, "    %s --> %s\n", from, to)
			continue
		}

		fmt.Fprintf(out, "    %s --> %s: %s\n", from, to, edge.label)
	}

	out.WriteString("```\n")

	return out.String(), nil
}
