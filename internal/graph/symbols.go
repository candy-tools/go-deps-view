package graph

import (
	"fmt"
	"go/types"
	"sort"

	"golang.org/x/tools/go/packages"
)

// Symbol is one exported package-level declaration, or an exported method of an
// exported type.
type Symbol struct {
	Name      string `json:"name"`      // "Build", or "Graph.Sorted" for a method
	Signature string `json:"signature"` // Go-like declaration; types from other packages use their package name
}

// PackageSymbols is the exported API of one in-module package.
type PackageSymbols struct {
	ID      string   `json:"id"`      // the graph node's id
	Symbols []Symbol `json:"symbols"` // sorted by name, so a type's methods follow it
}

// SymbolUse says package From references a symbol of package To. It refines an
// import edge, so every use has a matching Edge in the Graph. Reaching a symbol
// through a value from a third package, without importing its package, is no use.
type SymbolUse struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Symbol string `json:"symbol"` // a Symbol name in To; a struct field counts as its type
}

// Symbols is the symbol-level detail on top of a Graph. It needs the module type
// checked, which is too slow for every Graph, so it is built separately.
type Symbols struct {
	Packages []PackageSymbols `json:"packages"`
	Uses     []SymbolUse      `json:"uses"`
}

// symbolRef is what a types.Object resolves to in the output.
type symbolRef struct {
	pkgPath string
	pkgID   string
	name    string
}

// BuildSymbols type checks the module in dir and returns each in-module package's
// exported API and which of those symbols the other packages reference. Packages
// whose import path contains any of the exclude substrings are dropped, as in Build.
// A package with type errors yields what could be resolved, with the errors printed
// to stderr, because Build succeeds on such a module too.
func BuildSymbols(dir string, exclude []string) (*Symbols, error) {
	module, err := modulePath(dir)
	if err != nil {
		return nil, err
	}
	pkgs, err := packages.Load(&packages.Config{
		Dir:  dir,
		Mode: packages.NeedName | packages.NeedImports | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
	}, "./...")
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}
	packages.PrintErrors(pkgs)

	inModule := moduleMatcher(module, exclude)
	s := &Symbols{Packages: []PackageSymbols{}, Uses: []SymbolUse{}}
	refs := map[types.Object]symbolRef{}
	for _, p := range pkgs {
		if inModule(p.PkgPath) {
			id := relID(module, p.PkgPath)
			s.Packages = append(s.Packages, PackageSymbols{ID: id, Symbols: exportedSymbols(p.Types, id, refs)})
		}
	}
	// Separate pass: a package can be listed before the packages it imports.
	for _, p := range pkgs {
		if inModule(p.PkgPath) {
			s.Uses = append(s.Uses, symbolUses(p, relID(module, p.PkgPath), refs)...)
		}
	}

	// Outside a module, neither `go list -m` nor Load fails: the pattern's error is
	// only attached to a placeholder package, which inModule rejects.
	if len(s.Packages) == 0 {
		return nil, fmt.Errorf("no packages found in %s", dir)
	}

	sort.Slice(s.Packages, func(i, j int) bool { return s.Packages[i].ID < s.Packages[j].ID })
	sort.Slice(s.Uses, func(i, j int) bool {
		a, b := s.Uses[i], s.Uses[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		return a.Symbol < b.Symbol
	})
	return s, nil
}

// exportedSymbols lists pkg's exported API sorted by name, and records in refs what
// each of its objects resolves to. A struct's exported fields resolve to the struct
// itself, since they are part of its declaration rather than symbols of their own.
func exportedSymbols(pkg *types.Package, id string, refs map[types.Object]symbolRef) []Symbol {
	qualifier := func(other *types.Package) string {
		if other == pkg {
			return ""
		}
		return other.Name()
	}
	syms := []Symbol{}
	add := func(obj types.Object, name, signature string) {
		refs[obj] = symbolRef{pkgPath: pkg.Path(), pkgID: id, name: name}
		syms = append(syms, Symbol{Name: name, Signature: signature})
	}

	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		tn, isType := obj.(*types.TypeName)
		if !isType {
			add(obj, name, types.ObjectString(obj, qualifier))
			continue
		}
		add(obj, name, typeSignature(tn, qualifier))
		for _, m := range methodsOf(tn) {
			if m.Exported() {
				add(m, name+"."+m.Name(), types.ObjectString(m, qualifier))
			}
		}
		if st, ok := tn.Type().Underlying().(*types.Struct); ok && !tn.IsAlias() {
			for f := range st.Fields() {
				refs[f] = symbolRef{pkgPath: pkg.Path(), pkgID: id, name: name}
			}
		}
	}
	sort.Slice(syms, func(i, j int) bool { return syms[i].Name < syms[j].Name })
	return syms
}

// methodsOf returns the methods declared on tn: an interface's explicit methods, so
// an embedded interface's belong to that interface, or a defined type's methods. An
// alias has none, since they belong to the type it names.
func methodsOf(tn *types.TypeName) []*types.Func {
	if tn.IsAlias() {
		return nil
	}
	var out []*types.Func
	if iface, ok := tn.Type().Underlying().(*types.Interface); ok {
		for m := range iface.ExplicitMethods() {
			out = append(out, m)
		}
		return out
	}
	if named, ok := tn.Type().(*types.Named); ok {
		for m := range named.Methods() {
			out = append(out, m)
		}
	}
	return out
}

// typeSignature shortens a struct or interface to its keyword: the full body would
// not fit, and its methods are listed on their own anyway.
func typeSignature(tn *types.TypeName, qualifier types.Qualifier) string {
	if tn.IsAlias() {
		return "type " + tn.Name() + " = " + types.TypeString(types.Unalias(tn.Type()), qualifier)
	}
	name := types.TypeString(tn.Type(), qualifier) // includes type parameters
	switch u := tn.Type().Underlying().(type) {
	case *types.Struct:
		return "type " + name + " struct"
	case *types.Interface:
		return "type " + name + " interface"
	default:
		return "type " + name + " " + types.TypeString(u, qualifier)
	}
}

// symbolUses lists the known symbols that p references in the packages it imports.
func symbolUses(p *packages.Package, id string, refs map[types.Object]symbolRef) []SymbolUse {
	seen := map[SymbolUse]bool{}
	for _, obj := range p.TypesInfo.Uses {
		ref, ok := refs[originOf(obj)]
		if !ok || ref.pkgID == id {
			continue
		}
		if _, imported := p.Imports[ref.pkgPath]; imported {
			seen[SymbolUse{From: id, To: ref.pkgID, Symbol: ref.name}] = true
		}
	}
	uses := make([]SymbolUse, 0, len(seen))
	for u := range seen {
		uses = append(uses, u)
	}
	return uses
}

// originOf maps a method or field of an instantiated generic type back to the
// generic declaration, which is the object recorded in refs.
func originOf(obj types.Object) types.Object {
	switch o := obj.(type) {
	case *types.Func:
		return o.Origin()
	case *types.Var:
		return o.Origin()
	}
	return obj
}
