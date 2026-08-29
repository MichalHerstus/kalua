-- sleepy.lua — multiple sleeps
function main()
  k.print("before sleep")
  k.sleep(100)
  k.print("after 100ms")
  k.sleep(50)
  k.print("after 50ms")
  k.quit()
end