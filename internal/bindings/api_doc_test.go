package bindings

import "testing"

// TestApiDocSync guards against drift between registerKnown (what the checker
// and runtime install) and apiDocs (what tooling advertises). Every
// implemented binding must have a doc, and every doc must describe a real
// binding. Pure namespaces (form, ctrl, table) are exempt.
func TestApiDocSync(t *testing.T) {
	docs := Docs()
	for name, group := range registerKnown {
		if Namespace(name) {
			continue
		}
		info, ok := docs[name]
		if !ok {
			t.Errorf("binding %q (%s) is registered but has no api_doc entry", name, group)
			continue
		}
		if info.Group != group {
			t.Errorf("binding %q: api_doc group %q != registry group %q", name, info.Group, group)
		}
		if info.Signature == "" {
			t.Errorf("binding %q: missing signature", name)
		}
	}
	for name := range docs {
		if Namespace(name) {
			continue
		}
		if _, ok := registerKnown[name]; !ok {
			t.Errorf("api_doc entry %q has no corresponding binding in registerKnown", name)
		}
	}

	types := []struct {
		name string
		info Info
	}{
		{"print", apiDocs["print"]},
		{"form.new", apiDocs["form.new"]},
		{"table.add_line", apiDocs["table.add_line"]},
		{"json_load", apiDocs["json_load"]},
		{"checksum", apiDocs["checksum"]},
	}
	for _, tc := range types {
		if tc.info.Name != tc.name {
			t.Errorf("api_doc[%q].Name = %q", tc.name, tc.info.Name)
		}
	}

	if got := len(KInfo()); got != len(KSets) {
		t.Errorf("KInfo() returned %d entries, want %d", got, len(KSets))
	}
	if got := len(GlobalsList()); got != len(Globals) {
		t.Errorf("GlobalsList() returned %d entries, want %d", got, len(Globals))
	}
}
