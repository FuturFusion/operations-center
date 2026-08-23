package redfish

import (
	"strconv"
	"strings"
	"sync"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
)

// messageRegistry resolves the message registry IDs of a BMC into the message
// text and resolution it left out of its error responses.
//
// The registries are fetched on first lookup, which for a BMC reporting proper
// messages never happens.
type messageRegistry struct {
	client *gofish.APIClient

	once sync.Once
	// messages holds the registry entries keyed by "<prefix>.<key>".
	messages map[string]schemas.MessageRegistryMessage
}

func newMessageRegistry(client *gofish.APIClient) *messageRegistry {
	return &messageRegistry{client: client}
}

// lookup returns the registry entry for the given message ID, or nil if the BMC
// does not publish one.
func (m *messageRegistry) lookup(messageID string) *schemas.MessageRegistryMessage {
	if m == nil {
		return nil
	}

	prefix, key, ok := splitMessageID(messageID)
	if !ok {
		return nil
	}

	m.once.Do(m.load)

	message, ok := m.messages[prefix+"."+key]
	if !ok {
		return nil
	}

	return &message
}

// load fetches the message registries the BMC publishes. Registries which
// cannot be fetched or parsed are skipped.
func (m *messageRegistry) load() {
	m.messages = map[string]schemas.MessageRegistryMessage{}

	registryFiles, err := m.client.Service.Registries()
	if err != nil {
		return
	}

	for _, registryFile := range registryFiles {
		uri := registryFileURI(registryFile)
		if uri == "" {
			continue
		}

		registry, err := schemas.GetMessageRegistry(m.client, uri)
		if err != nil {
			continue
		}

		for key, message := range registry.Messages {
			m.messages[registry.RegistryPrefix+"."+key] = message
		}
	}
}

// registryFileURI returns the location of the registry file, preferring the
// English one and only considering locations served by the BMC itself.
func registryFileURI(registryFile *schemas.MessageRegistryFile) string {
	uri := ""

	for _, location := range registryFile.Location {
		if location.URI == "" {
			continue
		}

		if !strings.HasPrefix(location.URI, "/") {
			continue
		}

		if strings.HasPrefix(strings.ToLower(location.Language), "en") {
			return location.URI
		}

		if uri == "" {
			uri = location.URI
		}
	}

	return uri
}

// splitMessageID splits a message ID into the prefix of the registry holding it
// and its key within that registry. The version information is ignored.
func splitMessageID(messageID string) (prefix string, key string, ok bool) {
	rest, key, found := lastCut(messageID, ".")
	if !found || key == "" {
		return "", "", false
	}

	prefix, _, found = strings.Cut(rest, ".")
	if !found || prefix == "" {
		return "", "", false
	}

	return prefix, key, true
}

// expandMessageArgs substitutes the arguments of a message into its text, where
// "%1" refers to the first argument.
func expandMessageArgs(message string, args []string) string {
	for i, arg := range args {
		message = strings.ReplaceAll(message, "%"+strconv.Itoa(i+1), arg)
	}

	return message
}
