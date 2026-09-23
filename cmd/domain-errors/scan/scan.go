// Package scan finds the user facing errors of the code base and the places,
// where one should be used, but is not.
//
// The domain-errors command turns that into the report of the places, which do
// not follow the conventions of doc/development/error-handling.md.
package scan

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

const (
	// DomainPkg holds the error kinds and the constructor of a user facing error.
	DomainPkg = "github.com/FuturFusion/operations-center/internal/domain"

	// APIPkg declares the reasons, which are reported to a client.
	APIPkg = "github.com/FuturFusion/operations-center/shared/api"

	// ResponsePkg turns an error into an HTTP response.
	ResponsePkg = "github.com/FuturFusion/operations-center/internal/util/response"

	// SmartError is the response helper, which derives the status code from the
	// kind of the error. An error without a kind becomes an internal server
	// error there, which is what makes an unclassified error visible.
	SmartError = "SmartError"

	// IgnoreDirective exempts the call on the following line from the checks.
	// It carries the reason, so an exemption, which is no longer warranted, can
	// be told from one, which is.
	IgnoreDirective = "//domain-errors:ignore "

	// InternalDirective declares the error on the following line to be internal
	// by design, e.g. a violated invariant, which the user can do nothing
	// about. Such an error stays unclassified and is reported as an internal
	// server error.
	//
	// It is a directive of its own rather than an IgnoreDirective, because it
	// states the intent and makes every such place greppable.
	InternalDirective = "//domain-errors:internal "
)

// Result is everything the scan found in the inspected packages.
type Result struct {
	// Errors are the domain.NewErrorf calls, in the order they were found.
	Errors []DomainError

	// KindWraps are the calls, which classify an error by handing a domain
	// error kind to something other than domain.NewErrorf.
	KindWraps []KindWrap

	// BoundaryErrors are the errors, which are created at the HTTP boundary
	// without a kind, so they reach the user as an internal server error.
	BoundaryErrors []BoundaryError

	// BareKinds are the places, which return a kind without a message of its
	// own, outside the layer, where a kind is a sentinel for the caller.
	BareKinds []BareKind

	// UnkindedErrors are the errors, which originate in the business logic
	// without a kind, so nothing says, whether they are meant for the user.
	UnkindedErrors []UnkindedError

	// UnknownConstructs are the constructors and the builder methods of the
	// domain package, which the scan does not know about, so the errors they
	// build would be checked by nothing.
	UnknownConstructs []UnknownConstruct

	// DeclaredReasons are the api.ErrorReason constants of shared/api.
	DeclaredReasons []Reason

	// UsedReasons are the reason constants referenced anywhere in the module,
	// keyed by the constant name.
	UsedReasons map[string]bool
}

// DomainError is a domain.NewErrorf call together with the chain of methods
// called on its result.
type DomainError struct {
	Pos token.Position

	// Kind is the name of the kind, e.g. "ErrNotFound". It is empty for an
	// error constructed with a nil kind, which carries a message for the user
	// without classifying it.
	Kind string

	// Reason is the name of the api.ErrorReason constant, e.g.
	// "ErrorReasonServerIsClusterMember", or empty, if the empty string was
	// passed. ReasonValue is the value of the constant.
	Reason      string
	ReasonValue string

	// ReasonUnresolved is set, if the reason is neither the empty string, nor a
	// constant declared in shared/api, nor an api.ErrorReason resolved
	// elsewhere.
	ReasonUnresolved bool

	// Message is the format string of the message for the user. Format is
	// false, if it is not a constant, in which case the message can not be
	// reported in the catalog.
	Message      string
	MessageConst bool

	// Hint is the format string of the hint, if WithHintf is called on the
	// error. HintConst is false, if it is not a constant.
	Hint      string
	HintConst bool
	HasHint   bool

	// DetailKeys are the keys passed to WithDetail, in the order they are
	// added. A key, which is not a constant, is reported as the empty string.
	DetailKeys []string

	// CauseIsKind is set, if the error passed to WithCause is a domain error
	// kind itself, which reclassifies the error.
	CauseIsKind bool
	CauseKind   string

	// Ignored is set, if the call carries an ignore directive.
	Ignored bool
}

// KindWrap is a call, which hands a domain error kind to something other than
// domain.NewErrorf, e.g. fmt.Errorf("...: %w", domain.ErrNotFound).
type KindWrap struct {
	Pos token.Position

	// Callee is the function the kind is handed to, e.g. "fmt.Errorf".
	Callee string

	// Kind is the name of the kind, e.g. "ErrNotFound".
	Kind string

	// Ignored is set, if the call carries an ignore directive.
	Ignored bool
}

// BoundaryError is an error created at the HTTP boundary without a kind.
type BoundaryError struct {
	Pos token.Position

	// Responder is the response helper the error is handed to, e.g.
	// "response.BadRequest".
	Responder string

	// Callee is the function creating the error, e.g. "errors.New".
	Callee string

	// Ignored is set, if the call carries an ignore directive.
	Ignored bool
}

// BareKind is a kind returned as the error itself, without a message.
type BareKind struct {
	Pos token.Position

	// Kind is the name of the kind, e.g. "ErrNotFound".
	Kind string

	// Ignored is set, if the call carries an ignore directive.
	Ignored bool
}

// UnkindedError is an error, which originates where it is built, i.e. it wraps
// nothing, and carries no kind.
type UnkindedError struct {
	Pos token.Position

	// Callee is the function creating the error, e.g. "fmt.Errorf".
	Callee string

	// Message is the constant format string of the error.
	Message string

	// Internal is set, if the error is declared internal by design.
	Internal bool

	// Ignored is set, if the call carries an ignore directive.
	Ignored bool
}

// UnknownConstruct is a way of building a user facing error, which the scan
// does not understand.
type UnknownConstruct struct {
	Pos token.Position

	// Name is the name of the function or the method, e.g. "NewSomethingErr".
	Name string

	// What says, what the name is, so the report can tell the reader, where to
	// teach the scan about it.
	What string
}

// Reason is an api.ErrorReason constant declared in shared/api.
type Reason struct {
	Pos   token.Position
	Name  string
	Value string
}

// LoadMode is what the scan needs from the packages it inspects. The types of
// the whole program are needed, because a kind is recognised by the object it
// resolves to, not by the way it is spelled at the call site.
const LoadMode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports

// Load loads the given patterns, including their test files, so an error
// reported only by a test is covered as well.
func Load(patterns ...string) ([]*packages.Package, error) {
	pkgs, err := packages.Load(&packages.Config{Mode: LoadMode, Tests: false}, patterns...)
	if err != nil {
		return nil, fmt.Errorf("Failed to load the packages: %w", err)
	}

	var errs []string

	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		for _, pkgErr := range pkg.Errors {
			errs = append(errs, pkgErr.Error())
		}
	})

	if len(errs) > 0 {
		return nil, fmt.Errorf("Failed to type check the packages: %s", strings.Join(errs, "; "))
	}

	return pkgs, nil
}

// infrastructureSegments mark the layer, which talks to the database, the file
// system or another server. A kind returned there is a sentinel, which the
// service above translates into an error for the user, e.g. the ErrNotFound of
// a repository, which the service turns into "Server %q not found".
var infrastructureSegments = []string{"/repo/", "/adapter/", "/internal/adapter/", "/internal/sql/"}

// isInfrastructure reports whether the package belongs to the layer, which is
// allowed to hand a bare kind to its caller.
func isInfrastructure(pkgPath string) bool {
	// The prefixes are matched with the same leading separator as the segments.
	path := "/" + strings.TrimPrefix(pkgPath, "/")

	for _, segment := range infrastructureSegments {
		if strings.Contains(path, segment) {
			return true
		}
	}

	return false
}

// servicePackages returns the packages holding the business logic, which are
// the ones with a service in them. Deriving them from the loaded packages keeps
// a new service in scope without anybody having to remember a list, and it
// covers the files next to the service, e.g. server_deployment.go, which are
// business logic of the same package.
func servicePackages(pkgs []*packages.Package) map[string]bool {
	services := map[string]bool{}

	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if isInfrastructure(pkg.PkgPath) ||
			strings.HasSuffix(pkg.PkgPath, "/mock") || strings.HasSuffix(pkg.PkgPath, "/middleware") {
			return
		}

		for _, file := range pkg.GoFiles {
			if strings.HasSuffix(file, "_service.go") {
				services[pkg.PkgPath] = true

				return
			}
		}
	})

	return services
}

// constructorName matches the naming convention of the constructors of the
// domain package, e.g. NewErrorf, NewValidationErrf, NewRetryableErr.
var constructorName = regexp.MustCompile(`^New.*Err`)

// findUnknownConstructs reports the ways of building a user facing error, which
// the scan does not understand.
func findUnknownConstructs(pkgs []*packages.Package, result *Result) {
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if pkg.PkgPath != DomainPkg || pkg.Types == nil {
			return
		}

		scope := pkg.Types.Scope()

		for _, name := range scope.Names() {
			object := scope.Lookup(name)
			if !object.Exported() {
				continue
			}

			switch typed := object.(type) {
			case *types.Func:
				if !constructorName.MatchString(name) || knownConstructors[name] {
					continue
				}

				result.UnknownConstructs = append(result.UnknownConstructs, UnknownConstruct{
					Pos:  pkg.Fset.Position(typed.Pos()),
					Name: name,
					What: "constructor",
				})

			case *types.TypeName:
				if name != "Error" {
					continue
				}

				result.UnknownConstructs = append(result.UnknownConstructs, unknownBuilderMethods(pkg, typed)...)
			}
		}
	})
}

// unknownBuilderMethods reports the With methods of domain.Error, which
// domainError does not read.
func unknownBuilderMethods(pkg *packages.Package, typeName *types.TypeName) []UnknownConstruct {
	named, ok := typeName.Type().(*types.Named)
	if !ok {
		return nil
	}

	var unknown []UnknownConstruct

	for i := range named.NumMethods() {
		method := named.Method(i)

		if !method.Exported() || !strings.HasPrefix(method.Name(), "With") || knownBuilderMethods[method.Name()] {
			continue
		}

		unknown = append(unknown, UnknownConstruct{
			Pos:  pkg.Fset.Position(method.Pos()),
			Name: method.Name(),
			What: "builder method of domain.Error",
		})
	}

	return unknown
}

// Scan inspects the given packages. Generated files are inspected as well,
// because a generated error reaches the user just like a hand written one.
func Scan(pkgs []*packages.Package) (*Result, error) {
	result := &Result{UsedReasons: map[string]bool{}}
	services := servicePackages(pkgs)

	findUnknownConstructs(pkgs, result)

	for _, pkg := range pkgs {
		// A package without syntax is a dependency, which was only loaded to
		// type check the packages under inspection.
		if len(pkg.Syntax) == 0 {
			continue
		}

		for _, file := range pkg.Syntax {
			ignored, internal := directiveLines(pkg.Fset, file)

			s := &fileScan{
				pkg:            pkg,
				file:           file,
				ignored:        ignored,
				internal:       internal,
				infrastructure: isInfrastructure(pkg.PkgPath),
				service:        services[pkg.PkgPath],
				result:         result,
			}

			s.run()
		}
	}

	return result, nil
}

// fileScan carries the state of the inspection of a single file.
type fileScan struct {
	pkg      *packages.Package
	file     *ast.File
	ignored  map[int]bool
	internal map[int]bool
	result   *Result

	// infrastructure is set for a package, which may hand a bare kind to its
	// caller, service for a package holding business logic.
	infrastructure bool
	service        bool

	// stack are the nodes on the way from the file to the node currently
	// inspected, which is the last element.
	stack []ast.Node
}

func (s *fileScan) run() {
	s.collectReasons()

	ast.Inspect(s.file, func(node ast.Node) bool {
		if node == nil {
			s.stack = s.stack[:len(s.stack)-1]

			return false
		}

		s.stack = append(s.stack, node)

		switch n := node.(type) {
		case *ast.CallExpr:
			s.inspectCall(n)

		case *ast.ReturnStmt:
			s.inspectReturn(n)
		}

		return true
	})
}

// inspectReturn reports a kind handed to the caller as the error itself. It
// carries no message, so wherever it can reach the user, it shows up as the
// text of the kind alone, e.g. "Not found".
//
// In the infrastructure layer that is the intended sentinel: the repository
// does not know the name the service would put into the message, so the service
// translates the sentinel on the way out.
func (s *fileScan) inspectReturn(ret *ast.ReturnStmt) {
	if s.infrastructure {
		return
	}

	for _, result := range ret.Results {
		kind := s.kindName(result)
		if kind == "" {
			continue
		}

		s.result.BareKinds = append(s.result.BareKinds, BareKind{
			Pos:     s.position(ret),
			Kind:    kind,
			Ignored: s.ignored[s.position(ret).Line],
		})
	}
}

// errorConstructors build a new error out of what they are given. A kind handed
// to one of them classifies the error the old way, without a reason, a hint or
// details.
var errorConstructors = map[string]string{
	"fmt":    "Errorf",
	"errors": "Join",
}

func (s *fileScan) inspectCall(call *ast.CallExpr) {
	fn := s.calleeFunc(call)
	if fn == nil || fn.Pkg() == nil {
		return
	}

	if fn.Pkg().Path() == DomainPkg && fn.Name() == "NewErrorf" {
		s.result.Errors = append(s.result.Errors, s.domainError(call))

		// The kind handed to NewErrorf is the classification itself.
		return
	}

	if fn.Pkg().Path() == ResponsePkg && fn.Name() == SmartError {
		s.inspectSmartError(call)

		return
	}

	if errorConstructors[fn.Pkg().Path()] != fn.Name() {
		return
	}

	s.inspectUnkindedError(call, fn)

	for _, arg := range call.Args {
		kind := s.kindName(arg)
		if kind == "" {
			continue
		}

		s.result.KindWraps = append(s.result.KindWraps, KindWrap{
			Pos:     s.position(call),
			Callee:  fn.Pkg().Name() + "." + fn.Name(),
			Kind:    kind,
			Ignored: s.isIgnored(call),
		})
	}
}

// knownConstructors are the constructors of the domain package the scan
// understands. Each of them classifies an error on its own, so an error handed
// to one of them is not unclassified.
var knownConstructors = map[string]bool{
	"NewErrorf":         true,
	"NewRetryableErr":   true,
	"NewValidationErrf": true,
}

// knownBuilderMethods are the methods of domain.Error the scan reads in
// domainError. A method, which is missing here, carries whatever it carries
// past every rule.
var knownBuilderMethods = map[string]bool{
	"WithHintf":  true,
	"WithCause":  true,
	"WithDetail": true,
}

// inspectUnkindedError reports an error, which originates in the business logic
// and carries no kind, so nothing says whether it is meant for the user.
//
// Only an error, which wraps nothing, is reported: a wrap inherits the kind of
// what it wraps, and the "Failed to <operation>" context it adds is for the log.
// An error handed to a constructor of the domain package is classified by it.
func (s *fileScan) inspectUnkindedError(call *ast.CallExpr, fn *types.Func) {
	if !s.service || len(call.Args) == 0 {
		return
	}

	// errors.Join takes errors, not a message, so it never originates one.
	if fn.Name() == "Join" {
		return
	}

	message, isConst := s.stringValue(call.Args[0])
	if !isConst || strings.Contains(message, "%w") {
		return
	}

	if s.isClassifiedByCaller() {
		return
	}

	s.result.UnkindedErrors = append(s.result.UnkindedErrors, UnkindedError{
		Pos:      s.position(call),
		Callee:   fn.Pkg().Name() + "." + fn.Name(),
		Message:  message,
		Internal: s.isInternal(call),
		Ignored:  s.isIgnored(call),
	})
}

// isClassifiedByCaller reports whether the call currently inspected is an
// argument of something, which classifies it: a constructor of the domain
// package, e.g. domain.NewRetryableErr(fmt.Errorf(...)), or a response helper
// forcing a status code of its own, e.g. response.BadRequest(fmt.Errorf(...)).
func (s *fileScan) isClassifiedByCaller() bool {
	for i := len(s.stack) - 2; i >= 0; i-- {
		outer, ok := s.stack[i].(*ast.CallExpr)
		if !ok {
			continue
		}

		fn := s.calleeFunc(outer)
		if fn == nil || fn.Pkg() == nil {
			continue
		}

		switch fn.Pkg().Path() {
		case DomainPkg:
			if knownConstructors[fn.Name()] {
				return true
			}

		case ResponsePkg:
			// SmartError derives the status code from the error, so it
			// classifies nothing; every other helper sets one itself.
			if fn.Name() != SmartError {
				return true
			}
		}
	}

	return false
}

// inspectSmartError reports an error, which is created right where it is turned
// into a response, without a kind to classify it by. response.SmartError
// derives the status code from the kind, so such an error reaches the user as
// an internal server error, although the boundary knows exactly what is wrong
// with the request.
//
// The helpers, which force a status code of their own, e.g. response.BadRequest,
// are not reported. They do report the message to the user, they only miss the
// reason, the hint and the details.
func (s *fileScan) inspectSmartError(call *ast.CallExpr) {
	if len(call.Args) != 1 {
		return
	}

	inner, ok := call.Args[0].(*ast.CallExpr)
	if !ok {
		return
	}

	innerFn := s.calleeFunc(inner)
	if innerFn == nil || innerFn.Pkg() == nil {
		return
	}

	switch {
	case innerFn.Pkg().Path() == "errors" && innerFn.Name() == "New":
		// A new error never wraps anything, so it carries no kind.

	case innerFn.Pkg().Path() == "fmt" && innerFn.Name() == "Errorf":
		// An error wrapping another one inherits its kind, only an error
		// starting here is unclassified.
		format, isConst := s.stringValue(inner.Args[0])
		if !isConst || strings.Contains(format, "%w") {
			return
		}

	default:
		return
	}

	responseFn := s.calleeFunc(call)

	s.result.BoundaryErrors = append(s.result.BoundaryErrors, BoundaryError{
		Pos:       s.position(call),
		Responder: responseFn.Pkg().Name() + "." + responseFn.Name(),
		Callee:    innerFn.Pkg().Name() + "." + innerFn.Name(),
		Ignored:   s.isIgnored(call),
	})
}

// domainError reads a domain.NewErrorf call and the chain of methods called on
// its result, e.g. NewErrorf(...).WithHintf(...).WithDetail(...).
func (s *fileScan) domainError(call *ast.CallExpr) DomainError {
	domainErr := DomainError{
		Pos:     s.position(call),
		Ignored: s.isIgnored(call),
	}

	if len(call.Args) >= 1 {
		domainErr.Kind = s.kindName(call.Args[0])
	}

	if len(call.Args) >= 2 {
		domainErr.Reason, domainErr.ReasonValue, domainErr.ReasonUnresolved = s.reason(call.Args[1])
	}

	if len(call.Args) >= 3 {
		domainErr.Message, domainErr.MessageConst = s.stringValue(call.Args[2])
	}

	for _, chained := range s.chainedCalls(call) {
		sel, ok := chained.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}

		switch sel.Sel.Name {
		case "WithHintf":
			domainErr.HasHint = true
			if len(chained.Args) >= 1 {
				domainErr.Hint, domainErr.HintConst = s.stringValue(chained.Args[0])
			}

		case "WithDetail":
			key := ""
			if len(chained.Args) >= 1 {
				key, _ = s.stringValue(chained.Args[0])
			}

			domainErr.DetailKeys = append(domainErr.DetailKeys, key)

		case "WithCause":
			if len(chained.Args) >= 1 {
				kind := s.kindName(chained.Args[0])
				if kind != "" {
					domainErr.CauseIsKind = true
					domainErr.CauseKind = kind
				}
			}
		}
	}

	return domainErr
}

// chainedCalls returns the method calls applied to the result of call, from the
// innermost to the outermost. The chain is read off the stack of the inspection,
// where a step is spelled as CallExpr(SelectorExpr(previous step)).
func (s *fileScan) chainedCalls(call *ast.CallExpr) []*ast.CallExpr {
	// The call itself is the last element of the stack, its enclosing nodes
	// precede it.
	index := len(s.stack) - 1

	var chain []*ast.CallExpr

	current := ast.Node(call)

	for index >= 2 {
		sel, ok := s.stack[index-1].(*ast.SelectorExpr)
		if !ok || sel.X != current {
			break
		}

		next, ok := s.stack[index-2].(*ast.CallExpr)
		if !ok || next.Fun != sel {
			break
		}

		chain = append(chain, next)
		current = next
		index -= 2
	}

	return chain
}

// collectReasons records the api.ErrorReason constants declared in shared/api
// and every reference to one, so a reason, which is declared but never used,
// can be reported.
func (s *fileScan) collectReasons() {
	if s.pkg.PkgPath == APIPkg {
		for _, decl := range s.file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				continue
			}

			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}

				for _, name := range valueSpec.Names {
					obj, ok := s.pkg.TypesInfo.Defs[name].(*types.Const)
					if !ok || !isErrorReason(obj.Type()) {
						continue
					}

					s.result.DeclaredReasons = append(s.result.DeclaredReasons, Reason{
						Pos:   s.pkg.Fset.Position(name.Pos()),
						Name:  obj.Name(),
						Value: constantString(obj),
					})
				}
			}
		}
	}

	// A reference from within shared/api itself does not count as a use, the
	// declaration block would otherwise keep every reason alive.
	if s.pkg.PkgPath == APIPkg {
		return
	}

	ast.Inspect(s.file, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if !ok {
			return true
		}

		obj, ok := s.pkg.TypesInfo.Uses[ident].(*types.Const)
		if !ok || obj.Pkg() == nil || obj.Pkg().Path() != APIPkg || !isErrorReason(obj.Type()) {
			return true
		}

		s.result.UsedReasons[obj.Name()] = true

		return true
	})
}

// kindName returns the name of the domain error kind expr refers to, e.g.
// "ErrNotFound", or the empty string, if expr is not a kind.
//
// A kind is a package level variable of internal/domain, whose name starts with
// Err and which is an error. Resolving it through the type information rather
// than through the text of the expression keeps the check working for an
// aliased import and for a reference from within the domain package itself.
func (s *fileScan) kindName(expr ast.Expr) string {
	var ident *ast.Ident

	switch e := expr.(type) {
	case *ast.Ident:
		ident = e

	case *ast.SelectorExpr:
		ident = e.Sel

	default:
		return ""
	}

	obj, ok := s.pkg.TypesInfo.Uses[ident].(*types.Var)
	if !ok || obj.Pkg() == nil || obj.Pkg().Path() != DomainPkg {
		return ""
	}

	// A local variable happens to be named like a kind often enough to matter.
	if obj.Parent() != obj.Pkg().Scope() {
		return ""
	}

	if !strings.HasPrefix(obj.Name(), "Err") || !isError(obj.Type()) {
		return ""
	}

	return obj.Name()
}

// reason resolves the reason argument of domain.NewErrorf. The empty string is
// the sanctioned way of saying, that the reason is derived from the kind.
//
// A reason, which is not a constant but carries the api.ErrorReason type, is
// resolved by whoever passes it, e.g. a helper taking the reason as a
// parameter, so it is accepted.
func (s *fileScan) reason(expr ast.Expr) (name string, value string, unresolved bool) {
	literal, isConst := s.stringValue(expr)
	if isConst && literal == "" {
		return "", "", false
	}

	var ident *ast.Ident

	switch e := expr.(type) {
	case *ast.Ident:
		ident = e

	case *ast.SelectorExpr:
		ident = e.Sel

	default:
		return "", literal, !isErrorReason(s.pkg.TypesInfo.TypeOf(expr))
	}

	obj, ok := s.pkg.TypesInfo.Uses[ident].(*types.Const)
	if !ok || obj.Pkg() == nil || obj.Pkg().Path() != APIPkg || !isErrorReason(obj.Type()) {
		return "", literal, !isErrorReason(s.pkg.TypesInfo.TypeOf(expr))
	}

	return obj.Name(), constantString(obj), false
}

// stringValue returns the value of expr, if it is a constant string.
func (s *fileScan) stringValue(expr ast.Expr) (string, bool) {
	value := s.pkg.TypesInfo.Types[expr].Value
	if value == nil || value.Kind() != constant.String {
		return "", false
	}

	unquoted, err := strconv.Unquote(value.ExactString())
	if err != nil {
		return "", false
	}

	return unquoted, true
}

func (s *fileScan) calleeFunc(call *ast.CallExpr) *types.Func {
	var ident *ast.Ident

	switch fun := call.Fun.(type) {
	case *ast.Ident:
		ident = fun

	case *ast.SelectorExpr:
		ident = fun.Sel

	case *ast.IndexExpr:
		// A generic function, e.g. domain.NewErrorf, is instantiated
		// explicitly at some call sites.
		return s.calleeFunc(&ast.CallExpr{Fun: fun.X})

	case *ast.IndexListExpr:
		return s.calleeFunc(&ast.CallExpr{Fun: fun.X})

	default:
		return nil
	}

	fn, ok := s.pkg.TypesInfo.Uses[ident].(*types.Func)
	if !ok {
		return nil
	}

	return fn
}

func (s *fileScan) position(node ast.Node) token.Position {
	return s.pkg.Fset.Position(node.Pos())
}

func (s *fileScan) isIgnored(call *ast.CallExpr) bool {
	return s.ignored[s.position(call).Line]
}

func (s *fileScan) isInternal(call *ast.CallExpr) bool {
	return s.internal[s.position(call).Line]
}

// directiveLines returns the lines carrying a directive, which is the line
// following the directive comment.
func directiveLines(fset *token.FileSet, file *ast.File) (ignored map[int]bool, internal map[int]bool) {
	ignored = map[int]bool{}
	internal = map[int]bool{}

	for _, group := range file.Comments {
		for _, comment := range group.List {
			line := fset.Position(comment.Pos()).Line + 1

			switch {
			case strings.HasPrefix(comment.Text, IgnoreDirective):
				ignored[line] = true

			case strings.HasPrefix(comment.Text, InternalDirective):
				internal[line] = true
			}
		}
	}

	return ignored, internal
}

func isError(typ types.Type) bool {
	errorType, ok := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	if !ok {
		return false
	}

	return types.Implements(typ, errorType)
}

func isErrorReason(typ types.Type) bool {
	named, ok := typ.(*types.Named)
	if !ok {
		return false
	}

	obj := named.Obj()

	return obj.Name() == "ErrorReason" && obj.Pkg() != nil && obj.Pkg().Path() == APIPkg
}

func constantString(obj *types.Const) string {
	unquoted, err := strconv.Unquote(obj.Val().ExactString())
	if err != nil {
		return obj.Val().ExactString()
	}

	return unquoted
}
