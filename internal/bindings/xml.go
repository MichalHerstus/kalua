package bindings

import (
	"encoding/xml"
	"strings"

	"github.com/yuin/gopher-lua"
)

// xmlDocHandle represents a parsed XML document
type xmlDocHandle struct {
	root *xmlNode
}

// xmlNode represents a node in the XML tree
type xmlNode struct {
	Name     string
	Attrs    map[string]string
	Content  string
	Children []*xmlNode
	Parent   *xmlNode
}

// registerXML installs the XML bindings: k.xml_parse, k.xml_root, k.xml_child,
// k.xml_child_list, k.xml_attr, k.xml_content, k.xml_attrs, k.xml_name
func registerXML(e *Env) {
	// k.xml_parse(text) — parse XML and return document handle
	e.register("xml_parse", "xml", func(L *lua.LState) int {
		text := L.CheckString(1)
		root, err := parseXML(text)
		if err != nil {
			L.RaiseError("xml_parse: %v", err)
			return 0
		}
		handle := &xmlDocHandle{root: root}
		// Store handle as lightuserdata
		ud := L.NewUserData()
		ud.Value = handle
		L.Push(ud)
		return 1
	})

	// k.xml_root(doc) — get root element name
	e.register("xml_root", "xml", func(L *lua.LState) int {
		ud := L.CheckUserData(1)
		handle, ok := ud.Value.(*xmlDocHandle)
		if !ok || handle.root == nil {
			L.RaiseError("xml_root: invalid document handle")
			return 0
		}
		L.Push(lua.LString(handle.root.Name))
		return 1
	})

	// k.xml_child(doc, path) — get child element by path (e.g., "book/author")
	e.register("xml_child", "xml", func(L *lua.LState) int {
		ud := L.CheckUserData(1)
		handle, ok := ud.Value.(*xmlDocHandle)
		if !ok || handle.root == nil {
			L.RaiseError("xml_child: invalid document handle")
			return 0
		}
		path := L.CheckString(2)
		node := findNode(handle.root, path)
		if node == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(nodeToTable(L, node))
		return 1
	})

	// k.xml_child_list(doc, path) — get list of child elements
	e.register("xml_child_list", "xml", func(L *lua.LState) int {
		ud := L.CheckUserData(1)
		handle, ok := ud.Value.(*xmlDocHandle)
		if !ok || handle.root == nil {
			L.RaiseError("xml_child_list: invalid document handle")
			return 0
		}
		path := L.CheckString(2)
		parent := findNode(handle.root, path)
		if parent == nil {
			L.Push(lua.LNil)
			return 1
		}
		tbl := L.NewTable()
		for i, child := range parent.Children {
			tbl.RawSetInt(i+1, nodeToTable(L, child))
		}
		L.Push(tbl)
		return 1
	})

	// k.xml_attr(doc, path, attr_name) — get attribute value
	e.register("xml_attr", "xml", func(L *lua.LState) int {
		ud := L.CheckUserData(1)
		handle, ok := ud.Value.(*xmlDocHandle)
		if !ok || handle.root == nil {
			L.RaiseError("xml_attr: invalid document handle")
			return 0
		}
		path := L.CheckString(2)
		attrName := L.CheckString(3)
		node := findNode(handle.root, path)
		if node == nil {
			L.Push(lua.LNil)
			return 1
		}
		if val, ok := node.Attrs[attrName]; ok {
			L.Push(lua.LString(val))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})

	// k.xml_content(doc, path) — get text content of element
	e.register("xml_content", "xml", func(L *lua.LState) int {
		ud := L.CheckUserData(1)
		handle, ok := ud.Value.(*xmlDocHandle)
		if !ok || handle.root == nil {
			L.RaiseError("xml_content: invalid document handle")
			return 0
		}
		path := L.CheckString(2)
		node := findNode(handle.root, path)
		if node == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(node.Content))
		return 1
	})

	// k.xml_attrs(doc, path) — get all attributes as table
	e.register("xml_attrs", "xml", func(L *lua.LState) int {
		ud := L.CheckUserData(1)
		handle, ok := ud.Value.(*xmlDocHandle)
		if !ok || handle.root == nil {
			L.RaiseError("xml_attrs: invalid document handle")
			return 0
		}
		path := L.CheckString(2)
		node := findNode(handle.root, path)
		if node == nil {
			L.Push(lua.LNil)
			return 1
		}
		tbl := L.NewTable()
		for k, v := range node.Attrs {
			tbl.RawSetString(k, lua.LString(v))
		}
		L.Push(tbl)
		return 1
	})

	// k.xml_name(doc, path) — get element name
	e.register("xml_name", "xml", func(L *lua.LState) int {
		ud := L.CheckUserData(1)
		handle, ok := ud.Value.(*xmlDocHandle)
		if !ok || handle.root == nil {
			L.RaiseError("xml_name: invalid document handle")
			return 0
		}
		path := L.CheckString(2)
		node := findNode(handle.root, path)
		if node == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(node.Name))
		return 1
	})
}

// parseXML parses XML text and returns the root node
func parseXML(text string) (*xmlNode, error) {
	decoder := xml.NewDecoder(strings.NewReader(text))
	var root *xmlNode
	var current *xmlNode
	var stack []*xmlNode

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			node := &xmlNode{
				Name:     t.Name.Local,
				Attrs:    make(map[string]string),
				Children: []*xmlNode{},
			}
			for _, attr := range t.Attr {
				node.Attrs[attr.Name.Local] = attr.Value
			}
			if current != nil {
				node.Parent = current
				current.Children = append(current.Children, node)
			} else {
				root = node
			}
			stack = append(stack, current)
			current = node
		case xml.EndElement:
			if len(stack) > 0 {
				current = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if current != nil {
				content := strings.TrimSpace(string(t))
				if content != "" {
					if current.Content == "" {
						current.Content = content
					} else {
						current.Content += content
					}
				}
			}
		}
	}

	return root, nil
}

// findNode finds a node by path (e.g., "book/author")
func findNode(root *xmlNode, path string) *xmlNode {
	if path == "" || path == "/" {
		return root
	}
	parts := strings.Split(path, "/")
	current := root
	for _, part := range parts {
		if part == "" {
			continue
		}
		found := false
		for _, child := range current.Children {
			if child.Name == part {
				current = child
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return current
}

// nodeToTable converts an xmlNode to a Lua table
func nodeToTable(L *lua.LState, node *xmlNode) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("name", lua.LString(node.Name))

	// Attributes
	attrTbl := L.NewTable()
	for k, v := range node.Attrs {
		attrTbl.RawSetString(k, lua.LString(v))
	}
	tbl.RawSetString("attrs", attrTbl)

	// Content
	tbl.RawSetString("content", lua.LString(node.Content))

	// Children as list
	childrenTbl := L.NewTable()
	for i, child := range node.Children {
		childrenTbl.RawSetInt(i+1, nodeToTable(L, child))
	}
	tbl.RawSetString("children", childrenTbl)

	return tbl
}
