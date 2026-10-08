package graph

// SemanticRole describes generic behavior without replacing a language's kind.
type SemanticRole string

const (
	RootRole        SemanticRole = "ROOT"
	ContainerRole   SemanticRole = "CONTAINER"
	SourceRole      SemanticRole = "SOURCE"
	DeclarationRole SemanticRole = "DECLARATION"
)

// RoleOf recognizes only combinations emitted by supported analyzers. Legacy
// snapshots without language metadata keep an unknown role until rescanned.
func RoleOf(node Node) (SemanticRole, bool) {
	switch node.Language {
	case "go":
		switch node.Kind {
		case Module:
			return RootRole, true
		case Package:
			return ContainerRole, true
		case File:
			return SourceRole, true
		case Function, Method, Struct, Interface, Type, Variable, Constant:
			return DeclarationRole, IsDeclaration(node.Kind)
		}
	case "python":
		switch node.Kind {
		case Package:
			return ContainerRole, true
		case File:
			return SourceRole, true
		case Function, Class, Method:
			return DeclarationRole, IsDeclaration(node.Kind)
		}
	}
	return "", false
}
