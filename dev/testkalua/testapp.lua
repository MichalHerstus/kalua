-- minimal KALUA app
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
