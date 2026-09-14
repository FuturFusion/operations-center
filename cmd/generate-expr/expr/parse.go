package expr

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/FuturFusion/operations-center/cmd/generate-expr/lex"
)

type Parser struct {
	localPkg string

	structs map[string]map[string]*Struct
	aliases map[string]string

	exprStructs map[string]*StructResult
	converters  []*ast.FuncDecl

	usedPkgs map[string]string

	structMappings map[string]string
}

type Struct struct {
	Type       *types.TypeName
	Pkg        *types.Package
	Underlying *types.Struct
}

type StructResult struct {
	Struct *Struct
	Decl   *ast.GenDecl
}

// NewParser creates a new code parser.
func NewParser(localPkg string, pkgs []*packages.Package, aliases map[string]string) (*Parser, error) {
	structs, err := findAllStructs(pkgs)
	if err != nil {
		return nil, err
	}

	return &Parser{
		localPkg:       localPkg,
		structs:        structs,
		exprStructs:    map[string]*StructResult{},
		converters:     []*ast.FuncDecl{},
		usedPkgs:       map[string]string{},
		aliases:        aliases,
		structMappings: map[string]string{},
	}, nil
}

// CopyStruct copies the given struct and all sub-structs if necessary, and creating converter functions.
func (p *Parser) CopyStruct(structName, targetFilePrefix string) error {
	p.exprStructs = map[string]*StructResult{}
	p.converters = []*ast.FuncDecl{}
	p.usedPkgs = map[string]string{}

	structDef, ok := p.structs[p.localPkg][structName]
	if !ok {
		return fmt.Errorf("Struct %q not found in any package", structName)
	}

	// Generate the struct.
	if !p.generateStruct(structDef) {
		return nil
	}

	// Generate the converters for each struct generated above.
	for exprName, result := range p.exprStructs {
		p.converters = append(p.converters, p.generateConverter(exprName, result.Struct))
	}

	fileDecl := &ast.File{
		Name:  ast.NewIdent(structDef.Pkg.Name()),
		Decls: []ast.Decl{},
	}

	for _, d := range p.exprStructs {
		fileDecl.Decls = append(fileDecl.Decls, d.Decl)
	}

	for _, d := range p.converters {
		fileDecl.Decls = append(fileDecl.Decls, d)
	}

	// Sort the slices as they were assigned from a map.
	sort.Slice(fileDecl.Decls, func(i, j int) bool {
		switch t1 := fileDecl.Decls[i].(type) {
		case *ast.GenDecl:
			switch t1 := t1.Specs[0].(type) {
			case *ast.TypeSpec:
				// t1 is a type decl, compare to t2.
				switch t2 := fileDecl.Decls[j].(type) {
				case *ast.GenDecl:
					typeSpec, ok := t2.Specs[0].(*ast.TypeSpec)
					if ok {
						// t2 is also a type decl, compare names.
						return t1.Name.String() < typeSpec.Name.String()
					}

				case *ast.FuncDecl:
					// t2 is a function so it loses.
					return true
				}
			}

		case *ast.FuncDecl:
			// t1 is a func, compare to t2.
			switch t2 := fileDecl.Decls[j].(type) {
			case *ast.GenDecl:
				// t2 is a type, so it wins.
				return false

			case *ast.FuncDecl:
				// t2 is also a func, compare names.
				return t1.Name.String() < t2.Name.String()
			}
		}

		return false
	})

	var buf bytes.Buffer
	err := printer.Fprint(&buf, token.NewFileSet(), fileDecl)
	if err != nil {
		return err
	}

	// Add the generated file warning.
	targetFile := targetFilePrefix + FilePrefix
	err = os.WriteFile(targetFile, []byte(GenerateComment+"\n\n"), 0o644)
	if err != nil {
		return err
	}

	outFile, err := os.OpenFile(targetFile, os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}

	defer outFile.Close()

	src := splitAssignmentLines(buf.Bytes())

	qualifiers, err := usedQualifiers(src)
	if err != nil {
		return fmt.Errorf("Failed to parse the generated code for %q: %w", structName, err)
	}

	imports := p.importBlock(qualifiers)

	// Insert the import block right after the package clause.
	idx := bytes.IndexByte(src, '\n')
	if len(imports) > 0 && idx >= 0 {
		src = slices.Concat(src[:idx+1], []byte("\n"), imports, src[idx+1:])
	}

	_, err = outFile.Write(src)
	if err != nil {
		return err
	}

	testFile := targetFilePrefix + TestFilePrefix
	err = os.WriteFile(testFile, fmt.Appendf([]byte{}, UnitTestTemplate, filepath.Base(p.localPkg), structName, structName, structName), 0o644)
	if err != nil {
		return err
	}

	return nil
}

// Determines the underlying type name of the given type and returns it as an expression.
// If the type is a struct, it will call `generateStruct` to produce a new struct type if necessary, and then returns that struct's expression.
func (p *Parser) parseType(t types.Type) ast.Expr {
	switch t := t.(type) {
	case *types.Named:
		_, isStruct := t.Obj().Type().Underlying().(*types.Struct)
		pkg := t.Obj().Pkg()
		typeName := t.Obj().Name()
		exprName := p.getExprName(typeName, pkg)
		if isStruct && p.structs[pkg.Path()] != nil {
			_, ok := p.exprStructs[exprName]
			if ok {
				return ast.NewIdent(exprName)
			}

			if p.generateStruct(p.structs[pkg.Path()][typeName]) {
				return ast.NewIdent(exprName)
			}
		}

		pkgName := pkg.Name()
		alias, ok := p.aliases[pkg.Path()]
		if ok {
			pkgName = alias
		}

		if pkg.Path() == p.localPkg {
			return ast.NewIdent(typeName)
		}

		p.usedPkgs[pkg.Path()] = pkgName

		return &ast.SelectorExpr{
			X:   &ast.Ident{Name: pkgName},
			Sel: &ast.Ident{Name: typeName},
		}

	case *types.Pointer:
		return &ast.StarExpr{X: p.parseType(t.Elem())}

	case *types.Slice:
		return &ast.ArrayType{Elt: p.parseType(t.Elem())}

	case *types.Basic:
		return ast.NewIdent(t.Name())

	case *types.Map:
		return &ast.MapType{Key: p.parseType(t.Key()), Value: p.parseType(t.Elem())}

	case *types.Alias:
		return ast.NewIdent(t.Obj().Name())

	default:
		panic(fmt.Sprintf("Unexpected types.Type: %#v", t))
	}
}

// Generate a struct if the tag needs to be overwritten.
func (p *Parser) generateStruct(strct *Struct) bool {
	if strct == nil {
		return false
	}

	exprName := p.getExprName(strct.Type.Name(), strct.Pkg)
	if p.structMappings[strct.Type.Type().String()] == exprName {
		// If we already generated an expr struct for this struct for another invocation in the same package, then we can re-use it.
		return true
	}

	var overwroteAnyTag bool
	_, ok := p.exprStructs[exprName]
	if !ok {
		// place an empty decl before calling parseType to handle recursive types.
		p.exprStructs[exprName] = nil
		defer func() {
			if !overwroteAnyTag {
				delete(p.exprStructs, exprName)
			}
		}()
	}

	fields := make([]*ast.Field, 0, strct.Underlying.NumFields())
	for i := 0; i < strct.Underlying.NumFields(); i++ {
		field := strct.Underlying.Field(i)
		var names []*ast.Ident
		if !field.Embedded() {
			names = []*ast.Ident{ast.NewIdent(field.Name())}
		}

		tag, changed := overrideTag(strct.Underlying.Tag(i))
		if changed && !overwroteAnyTag {
			overwroteAnyTag = true
		}

		fields = append(fields, &ast.Field{
			Names: names,
			Type:  p.parseType(field.Type()),
			Tag:   &ast.BasicLit{Kind: token.STRING, Value: "`" + tag + "`"},
		})
	}

	if overwroteAnyTag {
		structDecl := &ast.GenDecl{
			Doc: &ast.CommentGroup{},
			Tok: token.TYPE,
			Specs: []ast.Spec{
				&ast.TypeSpec{
					Name: &ast.Ident{Name: exprName},
					Type: &ast.StructType{Fields: &ast.FieldList{List: fields}},
				},
			},
		}

		p.structMappings[strct.Type.Type().String()] = exprName
		p.exprStructs[exprName] = &StructResult{Decl: structDecl, Struct: strct}
	}

	return overwroteAnyTag
}

// Generates the ToExpr<Type> conversion helper for each created struct type.
func (p *Parser) generateConverter(exprName string, strct *Struct) *ast.FuncDecl {
	structName := strct.Type.Name()
	arg := strings.ToLower(string(structName[0]))
	if strct.Pkg.Path() != p.localPkg {
		structName = p.aliases[strct.Pkg.Path()] + "." + structName
		p.usedPkgs[strct.Pkg.Path()] = p.aliases[strct.Pkg.Path()]
	}

	assignFields := []ast.Expr{}
	for f := range strct.Underlying.Fields() {
		fieldName := f.Name()
		fType := f.Type()
		ptr, ok := f.Type().(*types.Pointer)
		if ok {
			fType = ptr.Elem()
		}

		switch t := fType.(type) {
		case *types.Map:
			assignFields = append(assignFields, &ast.KeyValueExpr{Key: ast.NewIdent(fieldName), Value: ast.NewIdent(p.recurseCollections(arg, f, t))})

		case *types.Slice:
			assignFields = append(assignFields, &ast.KeyValueExpr{Key: ast.NewIdent(fieldName), Value: ast.NewIdent(p.recurseCollections(arg, f, t))})

		case *types.Basic:
			assignFields = append(assignFields, &ast.KeyValueExpr{Key: ast.NewIdent(fieldName), Value: ast.NewIdent(arg + "." + fieldName)})

		case *types.Named:
			key := fieldName
			value := arg + "." + fieldName
			exprName, ok := p.structMappings[fType.String()]
			if ok {
				if f.Embedded() {
					key = exprName
				}

				if ptr != nil {
					value = "fromPtr(" + value + ")"
					value = fmt.Sprintf("toPtr(To%s(%s))", exprName, value)
				} else {
					value = fmt.Sprintf("To%s(%s)", exprName, value)
				}
			}

			assignFields = append(assignFields, &ast.KeyValueExpr{Key: ast.NewIdent(key), Value: ast.NewIdent(value)})
		}
	}

	// Add an additional line break at the end.
	if len(assignFields) > 0 {
		assignFields = append(assignFields, &ast.BasicLit{})
	}

	return &ast.FuncDecl{
		Doc:  &ast.CommentGroup{},
		Name: ast.NewIdent("To" + exprName),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{{
				Type:  ast.NewIdent(structName),
				Names: []*ast.Ident{ast.NewIdent(arg)},
			}}},
			Results: &ast.FieldList{List: []*ast.Field{{
				Type: ast.NewIdent(exprName),
			}}},
		},

		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ReturnStmt{Results: []ast.Expr{
				&ast.CompositeLit{Type: ast.NewIdent(exprName), Elts: assignFields},
			}},
		}},
	}
}

// Recursively calls <collection>Convert for each nested collection.
func (p *Parser) recurseCollections(arg string, f *types.Var, t types.Type) string {
	var elem types.Type
	var collection string
	switch t := t.(type) {
	case *types.Basic:
		return arg + "." + f.Name()

	case *types.Slice:
		collection = "slice"
		elem = t.Elem()

	case *types.Map:
		collection = "map"
		elem = t.Elem()

	case *types.Named:
		return arg + "." + f.Name()

	case *types.Pointer:
		return p.recurseCollections(arg, f, t.Elem())

	case *types.Alias:
		return t.Obj().Name()

	default:
		panic(fmt.Sprintf("Unexpected types.Type: %#v", t))
	}

	assignee := "x"
	if f.Type() == t {
		assignee = arg + "." + f.Name()
	}

	ptr, ok := elem.Underlying().(*types.Pointer)
	if ok {
		elem = ptr.Elem()
	}

	_, ok = elem.Underlying().(*types.Struct)
	if ok {
		exprName, ok := p.structMappings[elem.String()]
		if ok {
			funcName := "To" + exprName
			if ptr != nil {
				funcName = fmt.Sprintf("func(x *%s) *%s { return toPtr(%s(fromPtr(x))) }", filepath.Base(elem.String()), exprName, funcName)
			}

			return fmt.Sprintf("%sConvert(%s, %s)", collection, assignee, funcName)
		}

		return assignee
	}

	next := p.recurseCollections(arg, f, elem)
	if strings.Contains(next, "Convert(") {
		elemStr := elem.String()
		idx := strings.LastIndex(elemStr, "]")
		structType := elemStr[idx+1:]
		withoutPtr, ok := strings.CutPrefix(structType, "*")
		exprName := p.structMappings[withoutPtr]

		importIdx := strings.LastIndex(structType, ".")
		importPath := structType[:importIdx]
		if importPath == p.localPkg {
			structType = structType[importIdx+1:]
		} else {
			structType = filepath.Base(structType)
		}

		if ok {
			exprName = "*" + exprName
			structType = "*" + structType
		}

		elemStr = elemStr[:idx+1]

		if collection == "map" {
			parts := strings.Split(elemStr, "[")
			for i, p := range parts {
				parts[i] = filepath.Base(p)
			}

			elemStr = strings.Join(parts, "[")
		}

		return fmt.Sprintf(`%sConvert(%s, func(x %s) %s {
			return %s
})`, collection, assignee, elemStr+structType, elemStr+exprName, next)
	}

	return arg + "." + f.Name()
}

// Copies the existing `json` tag minus options into an `expr` tag if one does not already exist.
func overrideTag(existingTag string) (string, bool) {
	tag := existingTag
	if existingTag != "" {
		var newTag string
		parts := strings.Split(existingTag, " ")
		for _, tag := range parts {
			if strings.HasPrefix(tag, "expr:") {
				newTag = ""
				break
			}

			tagVal, ok := strings.CutPrefix(tag, "json:")
			if ok {
				tagVal, _, ok = strings.Cut(tagVal, ",")
				newTag = "expr:" + tagVal
				if ok {
					newTag += `"`
				}
			}
		}

		if newTag != "" {
			parts = append(parts, newTag)
			tag = strings.Join(parts, " ")
		}
	}

	return tag, tag != existingTag
}

// The naming structure of an expr defined struct. Format: `Expr<ExternalPackage><OriginalName>`.
func (p *Parser) getExprName(typeName string, pkg *types.Package) string {
	exprName := "Expr" + typeName
	if pkg.Path() != p.localPkg {
		exprName = "Expr" + lex.PascalCase(p.aliases[pkg.Path()]) + typeName
	}

	return exprName
}

// Finds all exported structs across all the packages in the given list.
func findAllStructs(pkgs []*packages.Package) (map[string]map[string]*Struct, error) {
	structs := map[string]map[string]*Struct{}
	for _, pkg := range pkgs {
		for _, decl := range pkg.Types.Scope().Names() {
			if structs[pkg.PkgPath] == nil {
				structs[pkg.PkgPath] = map[string]*Struct{}
			}

			_, ok := structs[pkg.PkgPath][decl]
			if ok {
				return nil, fmt.Errorf("Entity %q declaration exists more than once: %q (%q)", decl, pkg.PkgPath, structs[pkg.PkgPath][decl].Pkg.Path())
			}

			obj := pkg.Types.Scope().Lookup(decl)
			if !obj.Exported() {
				continue
			}

			structType, ok := obj.Type().Underlying().(*types.Struct)
			if !ok {
				continue
			}

			nameType, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}

			if !nameType.Exported() {
				continue
			}

			structs[pkg.PkgPath][decl] = &Struct{
				Type:       nameType,
				Pkg:        pkg.Types,
				Underlying: structType,
			}
		}
	}

	if len(structs) == 0 {
		return nil, fmt.Errorf("Failed to find any packages containing structs")
	}

	return structs, nil
}

func splitAssignmentLines(src []byte) []byte {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return src
	}

	// Collect positions where we need to insert blank lines.
	insertPositions := make([]int, 0, len(file.Decls))
	for _, d := range file.Decls {
		f, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}

		for _, s := range f.Body.List {
			s, ok := s.(*ast.ReturnStmt)
			if !ok {
				continue
			}

			for _, c := range s.Results {
				c, ok := c.(*ast.CompositeLit)
				if !ok {
					continue
				}

				// Record where the assignment starts.
				endPos := fset.Position(c.Lbrace)
				insertPositions = append(insertPositions, endPos.Offset)

				for _, kv := range c.Elts {
					// Record where each assignment ends.
					endPos := fset.Position(kv.End())
					insertPositions = append(insertPositions, endPos.Offset)
				}
			}
		}
	}

	// Insert newlines from end to start (to preserve offsets).
	result := src
	for i := len(insertPositions) - 1; i >= 0; i-- {
		pos := insertPositions[i]
		// Find next newline after declaration end.
		for pos < len(result) && !slices.Contains([]byte{',', '{'}, result[pos]) {
			pos++
		}

		if pos < len(result) {
			pos++ // Move past the newline.
			result = append(result[:pos], append([]byte("\n"), result[pos:]...)...)
		}
	}

	return result
}

// usedQualifiers returns the set of package qualifiers actually referenced in the
// given source. The generator emits some references as plain identifiers, so the
// printed source is re-parsed instead of inspecting the AST it was printed from.
func usedQualifiers(src []byte) (map[string]struct{}, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "", src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	qualifiers := map[string]struct{}{}
	ast.Inspect(file, func(n ast.Node) bool {
		selector, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		ident, ok := selector.X.(*ast.Ident)
		if ok {
			qualifiers[ident.Name] = struct{}{}
		}

		return true
	})

	return qualifiers, nil
}

// importBlock renders the import declaration for the file currently being
// generated, limited to the packages whose qualifier appears in qualifiers.
//
// The imports are split into the standard library, third party packages and
// packages of this module.
func (p *Parser) importBlock(qualifiers map[string]struct{}) []byte {
	const (
		groupStandard = iota
		groupThirdParty
		groupLocal
		groupCount
	)

	groups := make([][]string, groupCount)
	count := 0

	for path, qualifier := range p.usedPkgs {
		_, ok := qualifiers[qualifier]
		if !ok {
			continue
		}

		// Two packages can share a qualifier. Only one of them can be imported
		// without renaming the references in the generated code, so a package
		// that was explicitly passed with -p wins over a discovered one.
		_, declared := p.aliases[path]
		if !declared && p.declaresQualifier(qualifier) {
			continue
		}

		spec := `"` + path + `"`
		if qualifier != filepath.Base(path) {
			spec = qualifier + " " + spec
		}

		group := groupThirdParty
		switch {
		case !strings.Contains(strings.Split(path, "/")[0], "."):
			group = groupStandard

		case path == LocalModule || strings.HasPrefix(path, LocalModule+"/"):
			group = groupLocal
		}

		groups[group] = append(groups[group], spec)
		count++
	}

	if count == 0 {
		return nil
	}

	for _, group := range groups {
		slices.Sort(group)
	}

	if count == 1 {
		return fmt.Appendf(nil, "import %s\n", slices.Concat(groups...)[0])
	}

	var buf bytes.Buffer
	buf.WriteString("import (\n")
	separator := ""
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}

		buf.WriteString(separator)
		for _, spec := range group {
			fmt.Fprintf(&buf, "\t%s\n", spec)
		}

		separator = "\n"
	}

	buf.WriteString(")\n")

	return buf.Bytes()
}

// declaresQualifier reports whether one of the packages passed with -p is
// referenced through the given qualifier.
func (p *Parser) declaresQualifier(qualifier string) bool {
	for path, alias := range p.aliases {
		if alias == "" {
			alias = filepath.Base(path)
		}

		if alias == qualifier {
			return true
		}
	}

	return false
}
