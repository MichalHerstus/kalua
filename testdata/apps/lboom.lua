-- lboom.lua — Lua runtime error (index nil)
function main()
  local t = nil
  k.print(t.foo) -- attempt to index nil
end