package session

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yuin/gopher-lua"
)

func TestPagePayloadFromTable(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	ret := L.NewTable()
	dataTbl := L.NewTable()
	row1 := L.NewTable()
	row1.RawSetString("id", lua.LNumber(1))
	row1.RawSetString("name", lua.LString("Alice"))
	row2 := L.NewTable()
	row2.RawSetString("id", lua.LNumber(2))
	row2.RawSetString("name", lua.LString("Bob"))
	dataTbl.RawSetInt(1, row1)
	dataTbl.RawSetInt(2, row2)
	ret.RawSetString("data", dataTbl)
	ret.RawSetString("last_page", lua.LNumber(4))

	p := pagePayloadFromTable(ret)
	if p.LastPage != 4 {
		t.Errorf("last_page = %d, want 4", p.LastPage)
	}
	raw, ok := p.Data.(json.RawMessage)
	if !ok {
		t.Fatalf("Data type = %T, want json.RawMessage; p=%+v", p.Data, p)
	}
	if !strings.Contains(string(raw), `"Alice"`) || !strings.Contains(string(raw), `"Bob"`) {
		t.Errorf("data JSON missing rows: %s", raw)
	}
	if !strings.HasPrefix(string(raw), "[") || !strings.HasSuffix(string(raw), "]") {
		t.Errorf("data JSON not an array: %s", raw)
	}
}

func TestRemotePayloadToJSON(t *testing.T) {
	p := remotePagePayload{Data: json.RawMessage(`[{"id":1}]`), LastPage: 3}
	out := p.toJSON()
	if !strings.Contains(out, `"data":[{"id":1}]`) || !strings.Contains(out, `"last_page":3`) {
		t.Errorf("toJSON = %s, want data + last_page", out)
	}

	empty := remotePagePayload{}
	out = empty.toJSON()
	if !strings.Contains(out, `"data":[]`) {
		t.Errorf("empty toJSON = %s, want data:[]", out)
	}
}
