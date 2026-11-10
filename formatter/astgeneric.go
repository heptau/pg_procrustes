package formatter

import (
	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// walkChildren visits the child nodes of a node that walkNode has no case for, so that function
// calls, columns and aliases are found wherever the grammar puts an expression (a row constructor,
// an array, a window frame, a domain or trigger condition, a policy, a SQL-standard function body…)
// without listing every kind of node.
func walkChildren(node *pg_query.Node, pos *astPositions) {
	walkMessage(node.ProtoReflect(), pos)
}

func walkMessage(m protoreflect.Message, pos *astPositions) {
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsMap():
		case fd.IsList():
			if fd.Message() != nil {
				l := v.List()
				for i := 0; i < l.Len(); i++ {
					walkMessageValue(l.Get(i).Message(), pos)
				}
			}
		case fd.Message() != nil:
			walkMessageValue(v.Message(), pos)
		}
		return true
	})
}

func walkMessageValue(m protoreflect.Message, pos *astPositions) {
	if n, ok := m.Interface().(*pg_query.Node); ok {
		walkNode(n, pos)
		return
	}
	walkMessage(m, pos)
}
