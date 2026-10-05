package goanalyzer

import (
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"github.com/gabipuzon/nami/internal/graph"
)

func TestDeclarationFragment(t *testing.T) {
	source := `package example
const Max = 10
const (X = 1; Y = 2; _ = 3)
var current int
var a, b int
var _ = register()
type User struct{}
type Reader interface { Read() }
type UserID string
type Alias = struct{}
func Run() { value := 10; const local = 1; _ = value }
func (u User) Save() {}
func (u *User) Load() {}
func (u User[T]) Generic() {}
func init() {}
func init() {}
`
	file, err := parser.ParseFile(token.NewFileSet(), "example.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	first := declarationFragment(file, "example.go")
	second := declarationFragment(file, "example.go")
	if !reflect.DeepEqual(first, second) {
		t.Fatal("declarations changed between runs")
	}
	want := map[string]graph.NodeKind{
		"constant:example.go#Max": graph.Constant, "constant:example.go#X": graph.Constant,
		"constant:example.go#Y": graph.Constant, "variable:example.go#current": graph.Variable,
		"variable:example.go#a": graph.Variable, "variable:example.go#b": graph.Variable,
		"struct:example.go#User": graph.Struct, "interface:example.go#Reader": graph.Interface,
		"type:example.go#UserID": graph.Type, "type:example.go#Alias": graph.Type,
		"function:example.go#Run": graph.Function, "method:example.go#User.Save": graph.Method,
		"method:example.go#User.Load": graph.Method, "method:example.go#User.Generic": graph.Method,
		"function:example.go#init@1": graph.Function, "function:example.go#init@2": graph.Function,
	}
	if len(first.Nodes) != len(want) || len(first.Edges) != len(want) {
		t.Fatalf("nodes = %+v, edges = %+v", first.Nodes, first.Edges)
	}
	for i, node := range first.Nodes {
		if kind, ok := want[node.ID]; !ok || kind != node.Kind || node.Path != "example.go" || first.Edges[i] != (graph.Edge{Kind: graph.Contains, From: "file:example.go", To: node.ID}) {
			t.Fatalf("declaration = %+v, edge = %+v", node, first.Edges[i])
		}
		if node.Kind == graph.Function && node.Name == "init" {
			continue
		}
	}
	for _, node := range first.Nodes {
		if node.ID == "function:example.go#init@1" || node.ID == "function:example.go#init@2" {
			if node.Name != "init" {
				t.Fatalf("init name = %q", node.Name)
			}
		}
	}
}

func TestFileScopedDeclarationIdentity(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "thing_linux.go", "package thing\nfunc PlatformThing() {}", 0)
	if err != nil {
		t.Fatal(err)
	}
	linux := declarationFragment(file, "thing_linux.go")
	windows := declarationFragment(file, "thing_windows.go")
	if linux.Nodes[0].ID == windows.Nodes[0].ID {
		t.Fatal("declarations from different files collided")
	}
}
