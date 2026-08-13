package redfish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

// dumpEndpoints are the Redfish endpoints fetched by Dump. The paths follow
// the Redfish OpenAPI definition. Every "*" placeholder is replaced by the
// first member of the enclosing collection, therefore at most one member per
// collection is ever fetched: the primary purpose of the dump is to explore
// what a BMC is capable of, not to export all of its data.
var dumpEndpoints = []string{
	// Service root and root level services.
	schemas.DefaultServiceRoot,
	"/redfish/v1/odata",

	// Message registries, which hold the text of the messages BMCs report by
	// their registry ID alone.
	"/redfish/v1/Registries",
	"/redfish/v1/Registries/*",

	// Chassis.
	"/redfish/v1/Chassis",
	"/redfish/v1/Chassis/*",
	"/redfish/v1/Chassis/*/LogServices",
	"/redfish/v1/Chassis/*/LogServices/*",
	"/redfish/v1/Chassis/*/LogServices/*/Entries",
	"/redfish/v1/Chassis/*/LogServices/*/Entries/*",

	// Managers.
	"/redfish/v1/Managers",
	"/redfish/v1/Managers/*",
	"/redfish/v1/Managers/*/LogServices",
	"/redfish/v1/Managers/*/LogServices/*",
	"/redfish/v1/Managers/*/LogServices/*/Entries",
	"/redfish/v1/Managers/*/LogServices/*/Entries/*",
	"/redfish/v1/Managers/*/VirtualMedia",
	"/redfish/v1/Managers/*/VirtualMedia/*",

	// Systems.
	"/redfish/v1/Systems",
	"/redfish/v1/Systems/*",
	"/redfish/v1/Systems/*/Bios",
	"/redfish/v1/Systems/*/Bios/Settings",
	"/redfish/v1/Systems/*/BootOptions",
	"/redfish/v1/Systems/*/BootOptions/*",
	"/redfish/v1/Systems/*/LogServices",
	"/redfish/v1/Systems/*/LogServices/*",
	"/redfish/v1/Systems/*/LogServices/*/Entries",
	"/redfish/v1/Systems/*/LogServices/*/Entries/*",
	"/redfish/v1/Systems/*/Memory",
	"/redfish/v1/Systems/*/Memory/*",
	"/redfish/v1/Systems/*/Processors",
	"/redfish/v1/Systems/*/Processors/*",
	"/redfish/v1/Systems/*/SecureBoot",
	"/redfish/v1/Systems/*/SecureBoot/SecureBootDatabases",
	"/redfish/v1/Systems/*/SecureBoot/SecureBootDatabases/*",
	"/redfish/v1/Systems/*/VirtualMedia",
	"/redfish/v1/Systems/*/VirtualMedia/*",
}

func (r redfish) Dump(ctx context.Context, server provisioning.Server, additionalEndpoints []string, skipPredefined bool, trace bool) (api.BMCDump, error) {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	d := &dumper{client: client, trace: trace, dump: api.BMCDump{}}

	var endpoints []string
	if !skipPredefined {
		endpoints = append(endpoints, dumpEndpoints...)
	}

	endpoints = append(endpoints, additionalEndpoints...)

	for _, endpoint := range endpoints {
		uri, ok := d.resolve(endpoint)
		if !ok {
			continue
		}

		d.get(uri)
	}

	d.getActionInfos()

	return d.dump, nil
}

// getActionInfos fetches the action info resources referenced by the already
// dumped responses.
//
// An action info describes which parameters an action accepts, which of them
// are required and which values are allowed for them.
func (d *dumper) getActionInfos() {
	uris := map[string]struct{}{}

	for _, entry := range d.dump {
		if entry.Response == nil {
			continue
		}

		var body map[string]any

		err := json.Unmarshal(entry.Response, &body)
		if err != nil {
			continue
		}

		for _, uri := range actionInfoURIs(body) {
			uris[uri] = struct{}{}
		}
	}

	sortedURIs := make([]string, 0, len(uris))
	for uri := range uris {
		sortedURIs = append(sortedURIs, uri)
	}

	sort.Strings(sortedURIs)

	for _, uri := range sortedURIs {
		d.get(uri)
	}
}

// actionInfoURIs returns the "@Redfish.ActionInfo" targets of all actions
// declared in the "Actions" object of a Redfish resource.
func actionInfoURIs(body map[string]any) []string {
	actions, ok := body["Actions"].(map[string]any)
	if !ok {
		return nil
	}

	uris := make([]string, 0, len(actions))

	for name, rawAction := range actions {
		// "Oem" is the only member of "Actions" which is not an action itself.
		if name == "Oem" {
			continue
		}

		action, ok := rawAction.(map[string]any)
		if !ok {
			continue
		}

		uri, ok := action["@Redfish.ActionInfo"].(string)
		if !ok || uri == "" {
			continue
		}

		uris = append(uris, uri)
	}

	return uris
}

// dumper walks the dumpEndpoints templates against a connected Redfish
// client, resolving "*" placeholders to the first member of the
// enclosing collection and recording exactly one api.BMCDumpEntry per
// concrete URI it requests.
type dumper struct {
	client *gofish.APIClient
	trace  bool
	dump   api.BMCDump
}

// resolve turns a path template with "*" placeholders into a concrete
// URI by fetching each enclosing collection along the way and picking its
// first member. It returns false, if any collection along the path could not
// be fetched or has no members; the failing collection is already recorded as
// its own dump entry by get, so the caller can silently skip the template.
func (d *dumper) resolve(template string) (string, bool) {
	uri := ""
	for _, segment := range strings.Split(template, "/") {
		if segment == "" {
			continue
		}

		if segment != "*" {
			uri += "/" + segment
			continue
		}

		collection := d.get(uri)
		if collection == nil {
			return "", false
		}

		member, ok := firstMember(collection)
		if !ok {
			return "", false
		}

		uri = member
	}

	// Handle trailing slash for schemas.DefaultServiceRoot ("/redfish/v1/").
	if strings.HasSuffix(template, "/") && !strings.HasSuffix(uri, "/") {
		uri += "/"
	}

	return uri, true
}

// firstMember returns the "@odata.id" of the first member of a Redfish
// collection response, selecting deterministically by sorting the members'
// "@odata.id" values.
func firstMember(collection map[string]any) (string, bool) {
	rawMembers, ok := collection["Members"].([]any)
	if !ok || len(rawMembers) == 0 {
		return "", false
	}

	ids := make([]string, 0, len(rawMembers))
	for _, rawMember := range rawMembers {
		member, ok := rawMember.(map[string]any)
		if !ok {
			continue
		}

		id, ok := member["@odata.id"].(string)
		if !ok || id == "" {
			continue
		}

		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return "", false
	}

	sort.Strings(ids)

	return ids[0], true
}

// get issues a GET request for uri, records the result as an api.BMCDumpEntry
// and returns the decoded response body (nil on any failure). Every URI is
// only requested once; repeated calls return the cached result.
func (d *dumper) get(uri string) map[string]any {
	entry, ok := d.dump[uri]
	if ok {
		var body map[string]any

		_ = json.Unmarshal(entry.Response, &body)

		return body
	}

	var tw *traceWriter
	if d.trace {
		tw = &traceWriter{}
		d.client.SetDumpWriter(tw)
		defer d.client.SetDumpWriter(nil)
	}

	resp, err := d.client.Get(uri)

	entry = api.BMCDumpEntry{}
	if tw != nil {
		entry.Trace = tw.headersOnly()
	}

	if err != nil {
		entry.Error = newDumpError(err)
		d.dump[uri] = entry

		return nil
	}

	defer resp.Body.Close()

	entry.Allow = resp.Header.Get("Allow")

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		entry.Error = newDumpError(err)
		d.dump[uri] = entry

		return nil
	}

	entry.Response = json.RawMessage(body)
	d.dump[uri] = entry

	var decoded map[string]any

	err = json.Unmarshal(body, &decoded)
	if err != nil {
		return nil
	}

	return decoded
}

func newDumpError(err error) *api.BMCDumpError {
	dumpErr := &api.BMCDumpError{Error: err.Error(), Message: wrapRedfishError(err).Error()}

	var redfishErr *schemas.Error
	if errors.As(err, &redfishErr) {
		dumpErr.StatusCode = redfishErr.HTTPReturnedStatusCode
		dumpErr.Code = redfishErr.Code
	}

	return dumpErr
}
