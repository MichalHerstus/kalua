package bindings

import (
	"strings"
	"testing"

	glua "github.com/yuin/gopher-lua"

	"kalua/internal/vm"
)

// TestLayoutRender verifies the §11 layout controls (topbar/sidebar/footer)
// render their shell markup through the shared renderer.
func TestLayoutRender(t *testing.T) {
	L := glua.NewState()
	defer L.Close()
	e := &Env{L: L, known: map[string]string{}}

	formTbl := L.NewTable()
	controls := L.NewTable()

	// topbar
	userTbl := L.NewTable()
	userTbl.RawSetString("name", glua.LString("John Doe"))
	userTbl.RawSetString("role", glua.LString("admin"))
	userTbl.RawSetString("avatar", glua.LString("assets/avatar.png"))
	topbar := L.NewTable()
	topbar.RawSetString("name", glua.LString("bar"))
	topbar.RawSetString("type", glua.LString("topbar"))
	topbar.RawSetString("form", glua.LString("f"))
	topbar.RawSetString("icon", glua.LString("assets/logo.png"))
	topbar.RawSetString("title", glua.LString("My App"))
	topbar.RawSetString("user", userTbl)
	controls.RawSetString("bar", topbar)

	// sidebar (grid layout)
	items := L.NewTable()
	item1 := L.NewTable()
	item1.RawSetString("label", glua.LString("Dashboard"))
	item1.RawSetString("icon", glua.LString("assets/home.png"))
	item1.RawSetString("action", glua.LString("nav_dashboard"))
	items.RawSetInt(1, item1)
	sep := L.NewTable()
	sep.RawSetString("type", glua.LString("separator"))
	items.RawSetInt(2, sep)
	item3 := L.NewTable()
	item3.RawSetString("label", glua.LString("Settings"))
	item3.RawSetString("action", glua.LString("nav_settings"))
	item3.RawSetString("badge", glua.LNumber(5))
	items.RawSetInt(3, item3)
	style := L.NewTable()
	style.RawSetString("bg", glua.LString("#f8f9fa"))
	style.RawSetString("border_right", glua.LString("1px solid #e0e0e0"))
	sidebar := L.NewTable()
	sidebar.RawSetString("name", glua.LString("side"))
	sidebar.RawSetString("type", glua.LString("sidebar"))
	sidebar.RawSetString("form", glua.LString("f"))
	sidebar.RawSetString("position", glua.LString("left"))
	sidebar.RawSetString("width", glua.LNumber(280))
	sidebar.RawSetString("collapsed_width", glua.LNumber(64))
	sidebar.RawSetString("collapsible", glua.LTrue)
	sidebar.RawSetString("collapsed", glua.LFalse)
	sidebar.RawSetString("layout", glua.LString("grid"))
	sidebar.RawSetString("items", items)
	sidebar.RawSetString("style", style)
	controls.RawSetString("side", sidebar)

	// footer
	links := L.NewTable()
	link1 := L.NewTable()
	link1.RawSetString("label", glua.LString("Privacy"))
	link1.RawSetString("url", glua.LString("/privacy"))
	links.RawSetInt(1, link1)
	footer := L.NewTable()
	footer.RawSetString("name", glua.LString("foot"))
	footer.RawSetString("type", glua.LString("footer"))
	footer.RawSetString("form", glua.LString("f"))
	footer.RawSetString("text", glua.LString("© 2024 My Company"))
	footer.RawSetString("version", glua.LString("1.0.0"))
	footer.RawSetString("script_name", glua.LString("myapp.lua"))
	footer.RawSetString("links", links)
	controls.RawSetString("foot", footer)

	formTbl.RawSetString("controls", controls)
	L.SetGlobal("f", formTbl)

	e.known = map[string]string{"ctrl.topbar": "controls", "ctrl.sidebar": "controls", "ctrl.footer": "controls"}

	// topbar
	got := renderTopbar(topbar, "f", "bar", "c:f:bar", "")
	for _, want := range []string{
		`class="kalua-topbar"`, `id="c:f:bar"`, `data-k-topbar="1"`,
		`data-k-form="f"`, `data-k-ctrl="bar"`,
		`kalua-topbar-icon`, `src="assets/logo.png"`, `kalua-topbar-title`, `My App`,
		`data-k-topbar-user="1"`, `kalua-topbar-user-name`, `John Doe`,
		`kalua-topbar-user-role`, `admin`, `kalua-topbar-avatar`, `src="assets/avatar.png"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("renderTopbar missing %q: %s", want, got)
		}
	}

	// sidebar
	got = renderSidebar(sidebar, "f", "side", "c:f:side", "")
	for _, want := range []string{
		`class="kalua-sidebar"`, `id="c:f:side"`, `data-k-sidebar="1"`,
		`data-k-sidebar-position="left"`, `data-k-sidebar-width="280"`,
		`data-k-sidebar-collapsed-width="64"`, `data-k-sidebar-collapsible="true"`,
		`data-k-sidebar-collapsed="false"`, `data-k-sidebar-layout="grid"`,
		`data-k-sidebar-toggle="1"`, `class="kalua-sidebar-item"`,
		`data-k-sidebar-item="1"`, `data-k-sidebar-action="nav_dashboard"`,
		`data-k-sidebar-index="1"`, `Dashboard`, `kalua-sidebar-icon`,
		`class="kalua-sidebar-separator"`,
		`data-k-sidebar-action="nav_settings"`, `data-k-sidebar-index="2"`,
		`kalua-sidebar-badge`, `>5<`,
		`background:#f8f9fa`, `border-right:1px solid #e0e0e0`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("renderSidebar missing %q: %s", want, got)
		}
	}

	// footer
	got = renderFooter(footer, "f", "foot", "c:f:foot", "")
	for _, want := range []string{
		`class="kalua-footer"`, `id="c:f:foot"`, `data-k-footer="1"`,
		`kalua-footer-text`, `© 2024 My Company`, `kalua-footer-link`,
		`href="/privacy"`, `>Privacy<`, `kalua-footer-version`, `1.0.0`,
		`kalua-footer-script`, `myapp.lua`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("renderFooter missing %q: %s", want, got)
		}
	}
}

// TestSidebarVerticalLayout verifies a sidebar whose layout is "vertical"
// renders data-k-sidebar-layout="vertical", and that collapsible=false omits
// the collapse toggle.
func TestSidebarVerticalLayout(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	ctrl := L.NewTable()
	ctrl.RawSetString("name", glua.LString("side"))
	ctrl.RawSetString("type", glua.LString("sidebar"))
	ctrl.RawSetString("form", glua.LString("f"))
	ctrl.RawSetString("layout", glua.LString("vertical"))

	got := renderSidebar(ctrl, "f", "side", "c:f:side", "")
	if !strings.Contains(got, `data-k-sidebar-layout="vertical"`) {
		t.Errorf("renderSidebar vertical missing layout attr: %s", got)
	}
	if !strings.Contains(got, `data-k-sidebar-toggle="1"`) {
		t.Errorf("renderSidebar default collapsible should render toggle: %s", got)
	}

	// collapsible=false → no toggle.
	ctrl.RawSetString("collapsible", glua.LFalse)
	got = renderSidebar(ctrl, "f", "side", "c:f:side", "")
	if strings.Contains(got, `data-k-sidebar-toggle="1"`) {
		t.Errorf("renderSidebar collapsible=false still renders toggle: %s", got)
	}
}

// TestFooterVersionDefault verifies the footer falls back to the KALUA binary
// version when the script omits `version`, and omits script_name when unset
// (§11.3).
func TestFooterVersionDefault(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	ctrl := L.NewTable()
	ctrl.RawSetString("name", glua.LString("foot"))
	ctrl.RawSetString("type", glua.LString("footer"))
	ctrl.RawSetString("form", glua.LString("f"))

	got := renderFooter(ctrl, "f", "foot", "c:f:foot", "")
	if !strings.Contains(got, `class="kalua-footer-version"`) {
		t.Errorf("renderFooter missing version span: %s", got)
	}
	if strings.Contains(got, `class="kalua-footer-script"`) {
		t.Errorf("renderFooter should omit script_name when unset: %s", got)
	}
}

// TestLayoutControlScript exercises k.ctrl.topbar/sidebar/footer through the
// full installed binding surface: control tables carry the derived layout,
// handlers register the topbar logout fn, and renderForm tags the shell
// classes the CSS uses to pad the fixed bars.
func TestLayoutControlScript(t *testing.T) {
	L := setupFullState(t)

	src := `
k.form.new("main", { title = "Shell", layout = "grid",
  cells = { content = {width = 12} } })
k.ctrl.topbar("main", "bar", {
  icon = "assets/logo.png",
  title = "My App",
  user = { name = "John", role = "admin" },
  logout = function() _logged_out = true end,
})
k.ctrl.sidebar("main", "side", {
  position = "left",
  width = 280,
  collapsed_width = 64,
  items = {
    { label = "Dashboard", action = "nav_dashboard" },
    { type = "separator" },
    { label = "Settings", action = "nav_settings", badge = 5 },
  },
})
k.ctrl.footer("main", "foot", {
  text = "hello",
  script_name = "custom.lua",
})

local function assert(c, m) if not c then error(m) end end
local bar = CTRL("main", "bar")
assert(bar.type == "topbar", "topbar type")
assert(bar.title == "My App", "topbar title stored")
local side = CTRL("main", "side")
assert(side.type == "sidebar", "sidebar type")
assert(side.layout == "grid", "sidebar layout derived from form")
local foot = CTRL("main", "foot")
assert(foot.type == "footer", "footer type")
assert(foot.version ~= nil and foot.version ~= "", "footer version auto-filled")
assert(foot.script_name == "custom.lua", "footer script_name preserved")
-- The logout opt is registered as the control's "logout" handler.
local handlers = _G.main.handlers
assert(handlers.bar and handlers.bar.logout ~= nil, "logout handler registered")
__ok = true
`
	if err := L.DoString(src); err != nil {
		t.Fatalf("layout control script failed: %v", err)
	}
	if L.GetGlobal("__ok") != glua.LTrue {
		t.Fatal("script did not complete assertions")
	}

	// renderForm tags kalua-has-* classes for the fixed shell bars (§11.4) —
	// the sidebar is anchored to its configured side (left here) and the shell
	// controls render OUTSIDE the grid cells (direct children of .kalua-form).
	html := renderForm(L, "main")
	for _, want := range []string{
		`class="kalua-form kalua-has-topbar kalua-has-sidebar kalua-has-footer kalua-has-sidebar-left"`,
		`layout="grid"`,
		`--kalua-topbar-height:56px`, `--kalua-footer-height:40px`,
		`--kalua-sidebar-width:280px`, `--kalua-sidebar-collapsed-width:64px`,
		`data-k-sidebar-position="left"`, `data-k-sidebar-layout="grid"`,
		`data-k-topbar="1"`, `data-k-footer="1"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("renderForm missing %q: %s", want, html)
		}
	}
	// No sidebar cell exists (the sidebar is no longer a grid cell), and no
	// shell control is rendered inside a cell.
	if strings.Contains(html, `data-k-cell="sidebar"`) {
		t.Errorf("renderForm should not render a sidebar cell: %s", html)
	}
	if strings.Index(html, `class="kalua-cell"`) < 0 || strings.Index(html, `class="kalua-sidebar"`) < strings.Index(html, `class="kalua-cell"`) {
		t.Errorf("renderForm sidebar should render after (outside) the grid cells: %s", html)
	}
}

// TestShellLayoutRightSide verifies a sidebar with position="right" tags the
// form with kalua-has-sidebar-right and configures the same CSS variables.
func TestShellLayoutRightSide(t *testing.T) {
	L := setupFullState(t)

	src := `
k.form.new("main", { title = "Shell", layout = "vertical" })
k.ctrl.sidebar("main", "side", { position = "right", width = 240, collapsed_width = 48,
  items = { { label = "Home", action = "nav_home" } } })
k.ctrl.label("main", "lbl", { text = "hi" })
`
	if err := L.DoString(src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
	html := renderForm(L, "main")
	for _, want := range []string{
		`kalua-has-sidebar-right`,
		`--kalua-sidebar-width:240px`, `--kalua-sidebar-collapsed-width:48px`,
		`data-k-sidebar-position="right"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("renderForm missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, `kalua-has-sidebar-left`) {
		t.Errorf("renderForm should not tag left padding for a right sidebar: %s", html)
	}
	// The label stays in the normal flow; the sidebar is not rowwed.
	if i := strings.Index(html, `class="kalua-sidebar"`); i < 0 || strings.Index(html, "hi") > i {
		t.Errorf("vertical flow should list the label before the fixed shell: %s", html)
	}
}

// TestFooterScriptNameAutoFill verifies the footer auto-fills script_name from
// bindings.Options.ScriptName (§11.3).
func TestFooterScriptNameAutoFill(t *testing.T) {
	L := vm.New()
	defer L.Close()
	Setup(L, vm.NewApp(L), Options{ScriptName: "app.lua"}, nil, nil)
	if err := L.DoString(`
k.form.new("main", { title = "t" })
k.ctrl.footer("main", "foot", { text = "x" })
assert(_G.main.controls.foot.script_name == "app.lua", "script_name auto-filled")
`); err != nil {
		t.Fatalf("script_name auto-fill failed: %v", err)
	}
}
