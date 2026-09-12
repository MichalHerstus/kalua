-- ai_demo.lua — sample app produced by / resembles what the KALUA AI builder
-- generates: a run-mode form app with validation errors fed back for auto-fix.
-- Use it as a regression fixture: `KALUA check ai_demo.lua` must pass and the
-- app must run headless-free (interactive form).

function main()
  k.form.new("login", {
    title = "AI Builder Demo",
    layout = "vertical",
    align = "center",
  })

  k.ctrl.label("login", "head", {text = "Sign in to continue", multiline = true})
  k.ctrl.textbox("login", "username", {label = "Username", value = ""})
  k.ctrl.textbox("login", "password", {label = "Password"})
  k.ctrl.checkbox("login", "remember", {label = "Remember me", value = false})

  k.ctrl.button("login", "signin", {
    label = "Sign in",
    class = "kalua-button-primary",
    onclick = function()
      local user = k.ctrl.get_value("login", "username")
      local pass = k.ctrl.get_value("login", "password")
      if user == "" then
        k.msgbox{title = "Sign in", message = "Username is required", type = "warning"}
        return
      end
      if pass == "" then
        k.msgbox{title = "Sign in", message = "Password is required", type = "warning"}
        return
      end
      k.msgbox(string.format("Welcome, %s!", user))
      k.quit()
    end,
  })

  k.ctrl.button("login", "cancel", {
    label = "Cancel",
    onclick = function()
      k.form.close("login")
    end,
  })

  k.form.on("login", "key_pressed", function()
    -- Enter anywhere submits the form
  end)

  k.form.show("login")
end