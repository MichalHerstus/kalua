package host

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRun_JSONFlow(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "json.lua")
	src := fmt.Sprintf(`
function main()
  local base = %q

  local obj = k.json_parse('{"name":"kai","nums":[1,2,3],"nested":{"a":true,"b":null},"nothing":null}')
  if obj.name ~= "kai" then k.error("parse name") end
  if obj.nums[1] ~= 1 or obj.nums[3] ~= 3 then k.error("parse nums") end
  if not k.is_null(obj.nothing) then k.error("parse null not K.NULL") end

  if k.json_get(obj, "nested.a") ~= true then k.error("json_get bool") end
  if k.json_get(obj, "nums[1]") ~= 2 then k.error("json_get 0-based index") end

  if k.json_array_item(obj, "nums", 0) ~= 1 then k.error("array_item") end

  if k.json_count(obj, "nums") ~= 3 then k.error("count") end

  local names = k.json_names(obj, "")
  local hasName = false
  for _, name in ipairs(names) do
    if name == "name" then hasName = true end
  end
  if not hasName then k.error("names") end

  if not k.is_null(k.json_get(obj, "nothing")) then k.error("get null not K.NULL") end
  if k.is_null(obj.name) then k.error("is_null true on string") end

  local s = k.json_string(obj)
  local re = k.json_parse(s)
  if re.name ~= "kai" then k.error("stringify/parse roundtrip") end
  if not k.is_null(re.nested.b) then k.error("null not preserved through stringify") end
  if re.nested.b == nil then k.error("K.NULL should not be plain nil") end

  k.json_save(base .. "/data.json", obj)
  local fromFile = k.json_load(base .. "/data.json")
  if fromFile.name ~= "kai" then k.error("load name") end
  if fromFile.nums[2] ~= 2 then k.error("load nums") end
  if not k.is_null(fromFile.nothing) then k.error("load null") end

  k.quit()
end
`, tmp)
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	log := NewLogger(false)
	var buf bytes.Buffer
	cfg := RunConfig{ScriptPath: script, AllowFS: []string{tmp}, Logger: log, Out: &buf}
	code := Run(cfg)
	if code != ExitOK {
		t.Errorf("Run = %d, want %d\noutput:\n%s", code, ExitOK, buf.String())
	}
}
