-- KALUA shell demo — Topbar, Sidebar, Footer (kforms_enhancements.md §11).
-- Run: KALUA run testdata/apps/shell_demo.lua
-- Shows the three layout controls together: a fixed topbar with branding +
-- user info + logout, a collapsible sidebar driving a content textbox, and a
-- fixed footer with links and auto-filled version/script name.

function main()
  k.form.new("main", {
    title = "Shell Demo",
    layout = "grid",
    gap = 8,
  })

  -- Topbar: fixed header with branding + user + logout (§11.1).
  k.ctrl.topbar("main", "bar", {
    icon = "",
    title = "My App",
    user = {
      name = "John Doe",
      role = "admin",
      avatar = "",
    },
    logout = function()
      k.print("Topbar logout clicked")
    end,
  })

  -- Sidebar: fixed navigation panel anchored to the LEFT viewport edge,
  -- outside the form flow, collapsible to an icon strip (§11.2).
  k.ctrl.sidebar("main", "side", {
    position = "left",
    width = 280,
    collapsed_width = 64,
    collapsible = true,
    collapsed = false,
    items = {
      { label = "Dashboard", icon = "", action = "nav_dashboard" },
      { type = "separator" },
      { label = "Users", icon = "", action = "nav_users", badge = 5 },
      { label = "Reports", icon = "", action = "nav_reports" },
      { label = "Settings", icon = "", action = "nav_settings" },
    },
    on_select = function(item)
      k.print("sidebar on_select: " .. tostr(item and item.label))
    end,
  })

  k.ctrl.label("main", "heading", { text = "Content area" })
  k.ctrl.textbox("main", "content", { multiline = true, rows = 8, value = "" })
  k.ctrl.label("main", "hint", {
    text = "Click a sidebar item (→ on 'sidebar_select'), collapse the sidebar, or click the user block (→ 'logout').",
  })

  k.form.on("main", "side", "sidebar_select", function(action, item, index)
    k.print("sidebar_select:", action, index)
    k.ctrl.set_value("main", "content", "Selected " .. tostr((item and item.label) or action) .. " (item #" .. tostr(index) .. ")")
  end)

  k.form.on("main", "bar", "logout", function()
    k.print("topbar 'logout' event fired")
    k.ctrl.set_value("main", "content", "Logged out (demo)")
  end)

  -- Footer: fixed bottom bar with links and auto-filled version/script (§11.3).
  k.ctrl.footer("main", "foot", {
    text = "© 2024 My Company",
    links = {
      { label = "Privacy", url = "/privacy" },
      { label = "Terms", url = "/terms" },
    },
  })

  k.form.show("main")
end
