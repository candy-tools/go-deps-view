package graph

import (
	"path/filepath"
	"reflect"
	"testing"
)

// writeSymbolsModule writes a module where main imports mid and blank, and mid
// imports base. base declares one of each kind of symbol. main reaches Thing.Run
// through a value from mid without importing base.
func writeSymbolsModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module example.com/sym\n\ngo 1.25\n")
	write(t, filepath.Join(dir, "base", "base.go"), `package base

const Limit = 3

var Default = New()

type Thing struct {
	Name   string
	hidden int
}

func New() *Thing { return &Thing{hidden: Limit} }

func (t *Thing) Run() error { t.private(); return nil }
func (t *Thing) Stop()      {}
func (t *Thing) private()   {}

type Reader interface{ Read() string }

type Store interface {
	Reader
	Get(id string) string
}

type Box[T any] struct{ Value T }

func (b Box[T]) Get() T { return b.Value }

type Alias = Thing

type Mode int

type Unused struct{}

func helper() {}
`)
	write(t, filepath.Join(dir, "mid", "mid.go"), `package mid

import "example.com/sym/base"

func Make(s base.Store) *base.Thing {
	_ = s.Read()
	_ = s.Get("x")
	b := base.Box[int]{Value: 1}
	_ = b.Get()
	t := base.New()
	t.Name = "x"
	return t
}
`)
	write(t, filepath.Join(dir, "blank", "blank.go"), "package blank\n\nfunc Exported() {}\n")
	write(t, filepath.Join(dir, "main.go"), `package main

import (
	_ "example.com/sym/blank"
	"example.com/sym/mid"
)

func main() { _ = mid.Make(nil).Run() }
`)
	return dir
}

func TestBuildSymbolsDeclarations(t *testing.T) {
	s, err := BuildSymbols(writeSymbolsModule(t), nil)
	if err != nil {
		t.Fatalf("BuildSymbols: %v", err)
	}

	ids := []string{}
	var base []Symbol
	for _, p := range s.Packages {
		ids = append(ids, p.ID)
		if p.ID == "./base" {
			base = p.Symbols
		}
	}
	wantIDs := []string{".", "./base", "./blank", "./mid"}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Errorf("package ids = %v, want %v", ids, wantIDs)
	}

	want := []Symbol{
		{"Alias", "type Alias = Thing"},
		{"Box", "type Box[T any] struct"},
		{"Box.Get", "func (Box[T]).Get() T"},
		{"Default", "var Default *Thing"},
		{"Limit", "const Limit untyped int"},
		{"Mode", "type Mode int"},
		{"New", "func New() *Thing"},
		{"Reader", "type Reader interface"},
		{"Reader.Read", "func (Reader).Read() string"},
		{"Store", "type Store interface"},
		{"Store.Get", "func (Store).Get(id string) string"},
		{"Thing", "type Thing struct"},
		{"Thing.Run", "func (*Thing).Run() error"},
		{"Thing.Stop", "func (*Thing).Stop()"},
		{"Unused", "type Unused struct"},
	}
	if !reflect.DeepEqual(base, want) {
		t.Errorf("base symbols:\n got %+v\nwant %+v", base, want)
	}
}

func TestBuildSymbolsUses(t *testing.T) {
	s, err := BuildSymbols(writeSymbolsModule(t), nil)
	if err != nil {
		t.Fatalf("BuildSymbols: %v", err)
	}

	// A method resolves to the interface declaring it, a generic method to its
	// generic type, a field to its struct. Thing.Run is left out: main calls it but
	// doesn't import base. The blank import of blank references nothing.
	want := []SymbolUse{
		{From: ".", To: "./mid", Symbol: "Make"},
		{From: "./mid", To: "./base", Symbol: "Box"},
		{From: "./mid", To: "./base", Symbol: "Box.Get"},
		{From: "./mid", To: "./base", Symbol: "New"},
		{From: "./mid", To: "./base", Symbol: "Reader.Read"},
		{From: "./mid", To: "./base", Symbol: "Store"},
		{From: "./mid", To: "./base", Symbol: "Store.Get"},
		{From: "./mid", To: "./base", Symbol: "Thing"},
	}
	if !reflect.DeepEqual(s.Uses, want) {
		t.Errorf("uses:\n got %+v\nwant %+v", s.Uses, want)
	}
}

func TestBuildSymbolsTypeOfOtherPackageUsesPackageName(t *testing.T) {
	s, err := BuildSymbols(writeSymbolsModule(t), nil)
	if err != nil {
		t.Fatalf("BuildSymbols: %v", err)
	}
	for _, p := range s.Packages {
		if p.ID != "./mid" {
			continue
		}
		want := []Symbol{{"Make", "func Make(s base.Store) *base.Thing"}}
		if !reflect.DeepEqual(p.Symbols, want) {
			t.Errorf("mid symbols = %+v, want %+v", p.Symbols, want)
		}
	}
}

func TestBuildSymbolsExcludes(t *testing.T) {
	s, err := BuildSymbols(writeSymbolsModule(t), []string{"/base"})
	if err != nil {
		t.Fatalf("BuildSymbols: %v", err)
	}
	for _, p := range s.Packages {
		if p.ID == "./base" {
			t.Errorf("excluded package listed: %+v", p)
		}
	}
	want := []SymbolUse{{From: ".", To: "./mid", Symbol: "Make"}}
	if !reflect.DeepEqual(s.Uses, want) {
		t.Errorf("uses = %+v, want %+v", s.Uses, want)
	}
}

func TestBuildSymbolsEmptyCollectionsAreNonNil(t *testing.T) {
	s, err := BuildSymbols(writeMinimalModule(t), nil)
	if err != nil {
		t.Fatalf("BuildSymbols: %v", err)
	}
	if s.Uses == nil || len(s.Packages) != 1 || s.Packages[0].Symbols == nil {
		t.Fatalf("collections must be non-nil: %+v", s)
	}
}

// A type error must not take the whole view down: Build succeeds on such a module,
// so the rest of its symbols should still show.
func TestBuildSymbolsWithTypeError(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module example.com/broken\n\ngo 1.25\n")
	write(t, filepath.Join(dir, "good", "good.go"), "package good\n\nfunc Fine() {}\n")
	write(t, filepath.Join(dir, "bad", "bad.go"), `package bad

import "example.com/broken/good"

func Oops() int {
	good.Fine()
	return "not an int"
}
`)
	s, err := BuildSymbols(dir, nil)
	if err != nil {
		t.Fatalf("BuildSymbols: %v", err)
	}
	want := []SymbolUse{{From: "./bad", To: "./good", Symbol: "Fine"}}
	if !reflect.DeepEqual(s.Uses, want) {
		t.Errorf("uses = %+v, want %+v", s.Uses, want)
	}
}

func TestBuildSymbolsBadDir(t *testing.T) {
	if _, err := BuildSymbols(t.TempDir(), nil); err == nil {
		t.Fatal("expected an error for a non-module directory")
	}
}

// A package that can't resolve one of its imports still lists what it exports.
func TestBuildSymbolsWithMissingImport(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module example.com/missing\n\ngo 1.25\n")
	write(t, filepath.Join(dir, "a", "a.go"), "package a\n\nimport _ \"example.com/missing/nope\"\n\nfunc A() {}\n")
	s, err := BuildSymbols(dir, nil)
	if err != nil {
		t.Fatalf("BuildSymbols: %v", err)
	}
	if len(s.Uses) != 0 {
		t.Errorf("uses = %+v, want none", s.Uses)
	}
	if len(s.Packages) != 1 || len(s.Packages[0].Symbols) != 1 || s.Packages[0].Symbols[0].Name != "A" {
		t.Errorf("packages = %+v, want one exporting A", s.Packages)
	}
}
