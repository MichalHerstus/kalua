-- test_form.lua — test form and controls
function main()
  k.form.new("main", {title="Test Form", layout="vertical"})
  k.ctrl.label("main", "lbl1", {text="Hello KALUA!"})
  k.ctrl.textbox("main", "txt1", {label="Name", value="World"})
  k.ctrl.button("main", "btn1", {label="Click Me",
    onclick=function()
      local name = k.ctrl.get_value("main", "txt1")
      k.msgbox("Hello " .. name .. "!")
      k.quit()
    end})
  k.form.show("main")
end