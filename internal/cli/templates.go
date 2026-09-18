package cli

import (
	"fmt"
	"os"
	"strings"
)

// templates maps --template names to their script contents. The "run-form"
// name is the default for `kalua new` (no --template flag).
var templates = map[string]string{
	"run-form": `-- minimal KALUA app
function main()
  k.form.new("demo", {title = "Hello"})
  local lbl = k.ctrl.label("demo", "lbl", {label = "Enter your name"})
  local nameInput = k.ctrl.textbox("demo", "nameInput", {placeholder = "Name"})
  local greetBtn = k.ctrl.button("demo", "greetBtn", {label = "Greet"})
  k.form.on("demo", "greetBtn", "onclick", function()
    local name = k.ctrl.get_value("demo", "nameInput")
    if not name or name == "" then name = "stranger" end
    k.ctrl.set_property("demo", "lbl", "label", "Hello, " .. name .. "!")
  end)
  k.form.show("demo")
end
`,
	"run-crud": `-- KALUA CRUD app scaffold
function main()
  k.form.new("list", {title = "Items"})
  k.ctrl.label("list", "hdr", {label = "Items"})
  k.ctrl.button("list", "add", {label = "Add item"})
  k.form.on("list", "add", "onclick", function()
    k.form.show("add")
  end)
  k.form.show("list")
end

function handle_add()
  k.form.new("add", {title = "Add Item"})
  k.ctrl.textbox("add", "itemName", {placeholder = "Name"})
  k.ctrl.button("add", "save", {label = "Save"})
  k.ctrl.button("add", "back", {label = "Back"})
  k.form.on("add", "save", "onclick", function()
    k.form.close("add")
  end)
  k.form.on("add", "back", "onclick", function()
    k.form.close("add")
  end)
  k.form.show("add")
end
`,
	"serve-http": `-- serve-mode HTTP callback
function main() end

function handle_http(req)
  return {status = 200, body = "hello from KALUA"}
end
`,
	"serve-ws": `-- serve-mode WebSocket echo callback
function main() end

function handle_ws(msg)
  if msg.type == "text" then
    return "echo:" .. msg.data
  end
end
`,
	"serve-tcp": `-- serve-mode TCP echo callback
function main() end

function handle_tcp(msg)
  if msg.type == "text" then
    return "tcp-echo:" .. msg.data
  end
end
`,
	"serve-all": `-- serve-mode callbacks: HTTP + WebSocket + TCP
function main() end

function handle_http(req)
  return {status = 200, body = "hello from KALUA (HTTP)"}
end

function handle_ws(msg)
  if msg.type == "text" then
    return "echo:" .. msg.data
  end
end

function handle_tcp(msg)
  if msg.type == "text" then
    return "tcp-echo:" .. msg.data
  end
end
`,
}

// resolveScriptName derives the output path from the user-supplied name,
// ensuring the result ends in .lua and that an existing file is not
// silently overwritten.
func resolveScriptName(name string) (path string, err error) {
	if !strings.HasSuffix(name, ".lua") {
		path = name + ".lua"
	} else {
		path = name
	}
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already exists", path)
	}
	return path, nil
}

// selectTemplate returns the template body for the given --template name. An
// empty name defaults to "run-form".
func selectTemplate(name string) (string, error) {
	if name == "" {
		name = "run-form"
	}
	tmpl, ok := templates[name]
	if !ok {
		available := make([]string, 0, len(templates))
		for k := range templates {
			available = append(available, k)
		}
		return "", fmt.Errorf("unknown template %q (available: %s)",
			name, strings.Join(available, ", "))
	}
	return tmpl, nil
}
