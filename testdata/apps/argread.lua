-- argread.lua — reads ARGS table
function main()
  local n = #ARGS
  k.print("argc=" .. n)
  for i = 1, n do
    k.print("ARGS[" .. i .. "]=" .. ARGS[i])
  end
  k.quit()
end