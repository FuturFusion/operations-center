package authz

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Object is a string alias that represents an authorization object. These are formatted strings that
// uniquely identify an API resource, and can be constructed/deconstructed reliably.
// An Object is always of the form <ObjectType>:<identifier> where the identifier is a "/" delimited path containing elements that
// uniquely identify a resource. If the resource is defined at the project level, the first element of this path is always the project.
// Some example objects would be:
//   - `instance:default/c1`: Instance object in project "default" and name "c1".
//   - `storage_pool:local`: Storage pool object with name "local".
//   - `storage_volume:default/local/custom/vol1`: Storage volume object in project "default", storage pool "local", type "custom", and name "vol1".
type Object string

const (
	// objectTypeDelimiter is the string which separates the ObjectType from the remaining elements. Object types are
	// statically defined and do not contain this character, so we can extract the object type from an object by splitting
	// the string at this character.
	objectTypeDelimiter = ":"

	// objectElementDelimiter is the string which separates the elements of an object that make it a uniquely identifiable
	// resource. This was chosen because the character is not allowed in the majority of Incus resource names. Nevertheless
	// it is still necessary to escape this character in order to reliably construct/deconstruct an Object.
	objectElementDelimiter = "/"
)

// String implements fmt.Stringer for Object.
func (o Object) String() string {
	return string(o)
}

// Type returns the ObjectType of the Object.
func (o Object) Type() ObjectType {
	t, _, _ := strings.Cut(o.String(), objectTypeDelimiter)
	return ObjectType(t)
}

// Elements returns the elements that uniquely identify the authorization Object.
func (o Object) Elements() []string {
	_, identifier, _ := strings.Cut(o.String(), objectTypeDelimiter)

	escapedObjectComponents := strings.Split(identifier, objectElementDelimiter)
	components := make([]string, 0, len(escapedObjectComponents))
	for _, escapedComponent := range escapedObjectComponents {
		components = append(components, unescape(escapedComponent))
	}

	return components
}

// objectValidator contains fields that can be used to determine if a string is a valid Object.
type objectValidator struct {
	minIdentifierElements int
	maxIdentifierElements int
}

var objectValidators = map[ObjectType]objectValidator{
	ObjectTypeUser:   {minIdentifierElements: 1, maxIdentifierElements: 1},
	ObjectTypeServer: {minIdentifierElements: 1, maxIdentifierElements: 1},
}

// NewObject returns an Object of the given type. The passed in arguments must be in the correct
// order (as found in the URL for the resource). This function will error if an invalid object type is
// given, or if the correct number of arguments is not passed in.
func NewObject(objectType ObjectType, identifierElements ...string) (Object, error) {
	v, ok := objectValidators[objectType]
	if !ok {
		return "", fmt.Errorf("Missing validator for object of type %q", objectType)
	}

	if len(identifierElements) < v.minIdentifierElements {
		return "", fmt.Errorf("Authorization objects of type %q require at least %d components to be uniquely identifiable", objectType, v.minIdentifierElements)
	}

	if len(identifierElements) > v.maxIdentifierElements {
		return "", fmt.Errorf("Authorization objects of type %q require at most %d components to be uniquely identifiable", objectType, v.maxIdentifierElements)
	}

	builder := strings.Builder{}
	builder.WriteString(string(objectType))
	builder.WriteString(objectTypeDelimiter)

	for i, c := range identifierElements {
		builder.WriteString(escape(c))
		if i != len(identifierElements)-1 {
			builder.WriteString(objectElementDelimiter)
		}
	}

	return Object(builder.String()), nil
}

// ObjectFromRequest returns an object created from the request.
func ObjectFromRequest(r *http.Request, objectType ObjectType, muxVars ...string) (Object, error) {
	// Shortcut for server objects which don't require any arguments.
	if objectType == ObjectTypeServer {
		return ObjectServer(), nil
	}

	return "", errors.New("Only ObjectTypeServer is implemented right now")
}

// ObjectUser represents a user.
func ObjectUser(userName string) Object {
	object, _ := NewObject(ObjectTypeUser, userName)
	return object
}

// ObjectServer represents a server.
func ObjectServer() Object {
	object, _ := NewObject(ObjectTypeServer, "operations-center")
	return object
}

// escape escapes only the forward slash character as this is used as a delimiter. Everything else is allowed.
func escape(s string) string {
	return strings.ReplaceAll(s, "/", "%2F")
}

// unescape replaces only the escaped forward slashes.
func unescape(s string) string {
	return strings.ReplaceAll(s, "%2F", "/")
}
