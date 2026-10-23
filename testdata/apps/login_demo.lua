-- Login Control Demo (kforms_enhancements.md §8)
-- This demo creates a users table, seeds it with a test user,
-- and shows the login control in a modal form.

function main()
  -- Use the --db main=sqlite://... handle or create our own
  local db = k.connect_sqlite(":memory:")

  -- Create users table with pbkdf2 password
  k.sql(db, [[
        CREATE TABLE users (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            login TEXT UNIQUE NOT NULL,
            password TEXT NOT NULL,
            salt TEXT NOT NULL,
            name TEXT
        )
    ]])

  -- Insert a test user: login="ann", password="secret", salt="pepper"
  -- Using k.checksum to generate pbkdf2 hash
  local hash = k.checksum("pbkdf2", "secret", "pepper", 100000, 32)
  k.sql(db, "INSERT INTO users (login, password, salt, name) VALUES (?, ?, ?, ?)",
    "ann", hash, "pepper", "Ann Example")

  -- Also add a user with sha256 hash for testing different hash modes
  local sha256_hash = k.checksum("sha256", "password123", "staticsalt", 1, 32)
  k.sql(db, "INSERT INTO users (login, password, salt, name) VALUES (?, ?, ?, ?)",
    "bob", sha256_hash, "staticsalt", "Bob Builder")

  -- Create login form
  k.form.new("loginform", {
      layout = "vertical",
      align = "center"
    })

  -- Add login control
  k.ctrl.login("loginform", "login", {
      db = db,
      table = "users",
      login_column = "login",
      password_column = "password",
      salt_column = "salt",
      hash = "pbkdf2",
      iterations = 100000,
      keylen = 32,
      title = "User Login",
      subtitle = "Enter your credentials",
      login_label = "Login",
      password_label = "Password",
      login_placeholder = "Enter your login",
      password_placeholder = "Enter your password",
      submit_label = "Sign IN",
      cancel_label = "Cancel",
      error_message = "Invalid login or password",
      attempts = 3,
    })

  -- Handle successful login
  k.form.on("loginform", "login", "on_login", function(user)
      k.msgbox({
          title = "Welcome",
          message = "Hello, " .. user.name .. " (login: " .. user.login .. ")",
          type = "info",
        })
      k.form.close("loginform")
    end)

  -- Handle login error
  k.form.on("loginform", "login", "on_login_error", function(msg, left)
      k.print("Login error: " .. msg .. ", attempts left: " .. tostr(left))
    end)

  -- Handle cancel
  k.form.on("loginform", "login", "on_login_cancel", function()
      k.msgbox("Login cancelled")
      k.form.close("loginform")
    end)

  -- Show the form modally
  k.form.show("loginform", {modal = true})

  -- Clean up
  k.disconnect_db(db)
end
