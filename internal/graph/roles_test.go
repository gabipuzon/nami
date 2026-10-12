package graph

import "testing"

func TestSemanticRoles(t *testing.T) {
	for _, test := range []struct {
		language string
		kind     NodeKind
		role     SemanticRole
	}{
		{"go", Module, RootRole}, {"go", Package, ContainerRole}, {"go", File, SourceRole},
		{"go", Function, DeclarationRole}, {"go", Method, DeclarationRole}, {"go", Struct, DeclarationRole},
		{"go", Interface, DeclarationRole}, {"go", Type, DeclarationRole}, {"go", Variable, DeclarationRole}, {"go", Constant, DeclarationRole},
		{"python", Package, ContainerRole}, {"python", File, SourceRole}, {"python", Class, DeclarationRole},
		{"python", Function, DeclarationRole}, {"python", Method, DeclarationRole},
	} {
		t.Run(test.language+"/"+string(test.kind), func(t *testing.T) {
			role, ok := RoleOf(Node{Language: test.language, Kind: test.kind})
			if !ok || role != test.role {
				t.Fatalf("role=%q known=%v, want %q", role, ok, test.role)
			}
		})
	}
}

func TestUnknownSemanticRoles(t *testing.T) {
	for _, node := range []Node{
		{Kind: File}, {Kind: Class}, {Kind: Package, Language: "unknown"},
		{Kind: Module, Language: "python"}, {Kind: Struct, Language: "python"},
		{Kind: Class, Language: "go"}, {Kind: "CRATE", Language: "go"},
	} {
		if role, ok := RoleOf(node); ok || role != "" {
			t.Errorf("guessed role %q for %+v", role, node)
		}
	}
}

func TestTSJSRoles(t *testing.T) {
	for _, language := range []string{"typescript", "javascript"} {
		for _, kind := range []NodeKind{File, Function, Class, Method, Interface, Type, Variable, Enum} {
			role, ok := RoleOf(Node{Kind: kind, Language: language})
			if !ok || (kind == File && role != SourceRole) || (kind != File && role != DeclarationRole) {
				t.Fatal(language, kind, role, ok)
			}
		}
	}
}
