# Přehled funkcí k.*

Celkem: **221** funkcí. Popisy jsou v češtině; technické názvy (opts klíče, formáty) zůstávají v originále.

---

## Komunikace (FTP, sockety, SOAP)

| Funkce | Parametry | Popis |
|---|---|---|
| `k.ftp_connect` | `host[, port, user, pw]` | Připojí se k FTP serveru; vrací handle. |
| `k.ftp_create_dir` | `handle, path` | Vytvoří vzdálený adresář (MKD). |
| `k.ftp_delete` | `handle, path` | Smaže vzdálený soubor (DELE). |
| `k.ftp_disconnect` | `handle` | Odešle QUIT a zavře FTP připojení. |
| `k.ftp_file_exists` | `handle, path` | Zjistí, zda vzdálený soubor existuje (SIZE). |
| `k.ftp_get_file` | `handle, remote, local` | Stáhne vzdálený soubor do lokální cesty (RETR). |
| `k.ftp_list` | `handle[, path]` | Vypíše názvy vzdálených položek (LIST). |
| `k.ftp_put_file` | `handle, local, remote` | Nahraje lokální soubor na vzdálený server (STOR). |
| `k.ftp_rename` | `handle, from, to` | Přejmenuje vzdálený soubor nebo složku (RNFR/RNTO). |
| `k.ftp_set_cwd` | `handle, path` | Změní vzdálený pracovní adresář (CWD). |
| `k.socket_close` | `handle` | Zavře otevřený socket. |
| `k.socket_open` | `host, port[, timeout_ms]` | Otevře klientské TCP připojení; vrací handle. |
| `k.socket_read` | `handle[, count]` | Přečte ze socketu count bajtů (nebo vše až do uzavření). |
| `k.socket_read_line` | `handle` | Přečte jeden řádek (koncový nový řádek je odstraněn); nil na konci (EOF). |
| `k.socket_write` | `handle, data` | Zapíše data do otevřeného socketu; vrací počet zapsaných bajtů. |
| `k.webservice_run` | `profile, params` | Zavolá SOAP webovou službu. profile: {url, action[, method, timeout_ms]}; params je tabulka těla požadavku. Vrací {status, headers, body}. |
## Ovládací prvky

| Funkce | Parametry | Popis |
|---|---|---|
| `k.chart` | `—` | Operace s grafem (chart): k.chart.set_data/add_dataset/… |
| `k.chart.add_dataset` | `form, name, dataset` | Přidá datovou sadu {label, data, backgroundColor?, borderColor?, fill?, tension?, …} do grafu. |
| `k.chart.get_image` | `form, name` | Vykreslí plátno grafu a vrátí jej jako base64 PNG data URL. |
| `k.chart.remove_dataset` | `form, name, index` | Odebere datovou sadu podle indexu (1-založeného). |
| `k.chart.resize` | `form, name, width, height` | Změní velikost plátna grafu na zadané rozměry v pixelech. |
| `k.chart.set_data` | `form, name, {labels, datasets}` | Hromadně nahradí popisky (labels) a datové sady grafu. |
| `k.chart.set_labels` | `form, name, labels` | Nahradí popisky osy X grafu (pole). |
| `k.chart.set_options` | `form, name, options` | Spojí Chart.js options (scales, plugins, …) do grafu. |
| `k.chart.update_dataset` | `form, name, index, dataset` | Nahradí datovou sadu na 1-založeném indexu. |
| `k.ctrl` | `—` | Konstruktory ovládacích prvků: k.ctrl.label/textbox/button/… |
| `k.ctrl.button` | `form, name, optsTable` | Přidá ovládací prvek tlačítka. opts může nastavit label (text), class (CSS třídu), onclick (událost) a enabled (povoleno). |
| `k.ctrl.chart` | `form, name, optsTable` | Přidá Chart.js ovládací prvek. opts: {type=line|bar|hbar|pie|doughnut|scatter|radar|area, title, width=400, height=300, labels, datasets, options, responsive=true, maintainAspectRatio=false, legend=true, legendPosition=top, animation=true, stacked=false}. Události chart_click/chart_hover/chart_legend_click přes k.form.on. |
| `k.ctrl.checkbox` | `form, name, optsTable` | Přidá zaškrtávací pole (checkbox). |
| `k.ctrl.combo` | `form, name, optsTable` | Přidá rozevírací seznam (combo/dropdown). opts.items je tabulka voleb. |
| `k.ctrl.execute_event` | `form, name, event` | Spustí obsluhu události prvku, jako by ji vyvolal uživatel (např. "onclick"). Běží asynchronně přes session actor. |
| `k.ctrl.get_item_count` | `form, name` | Vrátí počet položek/řádků v prvku combo, list, radio nebo table. |
| `k.ctrl.get_property` | `form, name, prop` | Získá libovolnou vlastnost ovládacího prvku. |
| `k.ctrl.get_selection` | `form, name` | Vrátí aktuální výběr jako {start, end, text} z textboxu nebo textarea. |
| `k.ctrl.get_value` | `form, name` | Vrátí aktuální hodnotu ovládacího prvku. |
| `k.ctrl.grid` | `form, name, optsTable` | Přidá CRUD grid ovládací prvek: DB-propojenou Tabulator tabulku s akcemi řádků/globálními akcemi, výběrem a volitelným formulářem pro detail/úpravu. opts: {db, query, count_query?, page_size?, where?, order_by?, pk_field?, columns, row_actions?, global_actions?, selection_mode? (výchozí multi), row_click_action?, column_visibility?, form ("name" nebo inline {title, controls})}. Čtení stránkování/řazení/filtrů prochází přes Go host jako u tabulator=true tabulek; zápisy jsou zapojeny v dalších fázích. |
| `k.ctrl.image` | `form, name, optsTable` | Přidá ovládací prvek obrázku (<img>). opts: {src (povinné), alt, width, height (px nebo %), fit="cover|contain|fill|scale-down|none" (výchozí contain), clickable?, onclick?}. k.ctrl.set_value(form, name, new_src) aktualizuje obrázek. |
| `k.ctrl.label` | `form, name, optsTable` | Přidá ovládací prvek popisek (label). opts: {text, multiline?:boolean, cell?, align?}. multiline vykreslí div s white-space: pre-wrap zachovávající znaky nového řádku (\n). cell/align: přiřazení do grid rozložení + zarovnání. |
| `k.ctrl.list` | `form, name, optsTable` | Přidá víceřádkový výběrový seznam. opts.items je tabulka voleb. |
| `k.ctrl.looper` | `form, name, optsTable` | Přidá ovládací prvek looper (opakující se rozložení řádků). Propojený s DB, když opts obsahují {db, query, links, page_size?, count_query?, where?, order_by?}; db je handle z k.connect_db nebo --db NAME předregistrované při startu. S opts.row (pole {type, name, property?, field?|column?, opts?} definic řádkové šablony) jsou řádky vykreslovány na serveru jako skutečné prvky (label/textbox/image/checkbox, read-only); links lze vynechat — odvodí se z row. |
| `k.ctrl.radio` | `form, name, optsTable` | Přidá ovládací prvek radio (přepínací tlačítko). |
| `k.ctrl.refresh` | `form, name` | Znovu vykreslí jeden ovládací prvek a odešle aktualizaci. |
| `k.ctrl.select_text` | `form, name` | Vybere veškerý text v prvku textbox nebo textarea. |
| `k.ctrl.set_focus` | `form, name` | Přesune fokus na prvek v prohlížeči. |
| `k.ctrl.set_property` | `form, name, prop, value` | Nastaví libovolnou vlastnost ovládacího prvku. |
| `k.ctrl.set_selection` | `form, name, from, to` | Nastaví rozsah výběru v textboxu nebo textarea (0-založené offsety znaků). |
| `k.ctrl.set_value` | `form, name, value` | Nastaví hodnotu ovládacího prvku a znovu jej vykreslí. |
| `k.ctrl.table` | `form, name, optsTable` | Přidá ovládací prvek tabulky; řádky se manipulují přes k.table.*. S opts {db, query, …} je tabulka propojená s DB (Tabulator režim). db je handle z k.connect_db nebo --db NAME předregistrované při startu. |
| `k.ctrl.textbox` | `form, name, optsTable` | Přidá ovládací prvek textové pole. opts: {label, value, enabled, visible, multiline?:boolean, rows?:number, cols?:number, datetime?:boolean|table, cell?, align?}. multiline vykreslí <textarea>. datetime zapne flatpickr výběr: mode="date"|"time"|"datetime", format, min, max, step. cell/align: grid rozložení + zarovnání. |
| `k.grid` | `—` | Operace s grid ovládacím prvkem: k.grid.refresh/set_db_source/… |
| `k.grid.batch_delete` | `form, name, pksTable` | Smaže více řádků podle primárních klíčů (async). Při úspěchu spustí tabulator_refresh. Vrací boolean úspěch, nebo nil + chybovou zprávu. |
| `k.grid.delete_row` | `form, name, pk` | Smaže jeden řádek podle primárního klíče (async). Při úspěchu spustí tabulator_refresh. Vrací boolean úspěch, nebo nil + chybovou zprávu. |
| `k.grid.get_row` | `form, name, pk` | Získá jeden řádek podle primárního klíče (async). Čeká, dokud nedoběhne DB dotaz. Vrací objekt řádku (mapa sloupec→hodnota) nebo nil, pokud není nalezen. |
| `k.grid.get_selected` | `form, name` | Vrátí vybrané řádky (async). Čeká, dokud prohlížeč neodpoví daty vybraných řádků. Vrací seznam objektů řádků (každý je mapa sloupec→hodnota) nebo nil, pokud není vybráno. |
| `k.grid.insert_row` | `form, name, dataTable` | Vloží nový řádek (async). Vrací primární klíč vloženého řádku (pokud je k dispozici) nebo true při úspěchu, jinak nil + chybovou zprávu. |
| `k.grid.refresh` | `form, name` | Znovu spustí datový dotaz gridu a zobrazí stránku 1. |
| `k.grid.set_db_source` | `form, name, opts` | Vymění zdroj dat gridu {db, query, columns?, page_size?, count_query?, where?, order_by?, pk_field?, selection_mode?} a obnoví zobrazení. |
| `k.grid.update_row` | `form, name, pk, dataTable` | Aktualizuje řádek podle primárního klíče (async). Při úspěchu spustí tabulator_refresh. Vrací boolean úspěch, nebo nil + chybovou zprávu. |
| `k.looper` | `—` | Operace s looper ovládacím prvkem: k.looper.link_db/set_db_source/refresh/… |
| `k.looper.add_line` | `form, name, valuesTable` | Na looperu propojeném s DB vyvolá runtime chybu (řádky přicházejí z připojeného dotazu). |
| `k.looper.delete_line` | `form, name, index` | Na looperu propojeném s DB vyvolá runtime chybu (řádky přicházejí z připojeného dotazu). |
| `k.looper.link_db` | `form, name, opts` | Připojí k looperu DB zdroj: {db, query, links, page_size?, count_query?, where?, order_by?}. Seznam links mapuje sloupce výsledku na šablonové prvky: {column=N, control, property} podle 1-založeného indexu nebo {field, col, control, property} podle názvu. |
| `k.looper.refresh` | `form, name` | Znovu spustí dotaz looperu propojeného s DB a zobrazí stránku 1. |
| `k.looper.set_db_source` | `form, name, opts` | Vymění zdroj looperu propojeného s DB {db, query, links?, page_size?, count_query?, where?, order_by?} a obnoví zobrazení. |
| `k.looper.set_line` | `form, name, index, valuesTable` | Na looperu propojeném s DB vyvolá runtime chybu (řádky přicházejí z připojeného dotazu). |
| `k.table` | `—` | Operace s tabulkovým prvkem: k.table.add_line/delete_line/… |
| `k.table.add_line` | `form, name, valuesTable` | Přidá řádek do tabulkového prvku. |
| `k.table.delete_line` | `form, name, index` | Odebere řádek na indexu. |
| `k.table.find` | `form, name, value` | Prohledá řádky a najde ten, kde se kterákoli buňka rovná hodnotě. Vrací 1-založený index řádku nebo nil. Funguje pro tradiční i Tabulator tabulky. |
| `k.table.get_column_value` | `form, name, row, column` | Získá hodnotu buňky z tabulkového prvku. |
| `k.table.get_data` | `form, name` | Vrátí všechna aktuální data tabulkového prvku. |
| `k.table.get_selected_column` | `form, name` | Získá aktuálně vybraný sloupec. |
| `k.table.get_selected_rows` | `form, name` | Vrátí indexy vybraných řádků (1-založené). |
| `k.table.refresh` | `form, name` | Znovu spustí dotaz tabulky propojené s DB (Tabulator) a zobrazí stránku 1. |
| `k.table.set_column_value` | `form, name, row, column, value` | Nastaví hodnotu buňky v tabulkovém prvku. |
| `k.table.set_data` | `form, name, dataTable` | Hromadně nahradí všechna data řádků (v režimu Tabulator odešle tabulator_update). |
| `k.table.set_db_source` | `form, name, opts` | Vymění zdroj tabulky propojené s DB (Tabulator) {db, query, columns?, page_size?, count_query?, where?, order_by?} a obnoví zobrazení. |
| `k.table.set_remote_data` | `form, name, {data,last_page,last_row}` | Odešle data stránkování ze serveru do Tabulator tabulky: {data=rows, last_page=n} nebo {data=rows, last_row=n}. |
| `k.table.set_selected_column` | `form, name, column` | Nastaví vybraný sloupec. |
## Kryptografie a hashe

| Funkce | Parametry | Popis |
|---|---|---|
| `k.checksum` | `alg, data[, key[, salt[, iterations[, keylen]]]]` | Vypočítá hex hash pro alg: crc32, md5, sha1, sha256, hmac-sha256 (vyžaduje klíč), pbkdf2 (vyžaduje salt). |
| `k.crypt_asymmetric` | `alg, key, data` | RSA PKCS#1 v1.5 šifrování/dešifrování s PEM klíčem. |
| `k.crypt_symmetric` | `alg, key, data[, iv]` | Symetrické AES-CBC šifrování/dešifrování (alg aes-encrypt/aes-decrypt). Vrací base64. |
| `k.decrypt` | `b64, key` | Inverzní funkce ke k.encrypt. |
| `k.encrypt` | `plaintext, key` | AES-GCM šifrování; vrací base64(nonce || ciphertext). |
| `k.sign` | `data, key[, alg]` | Vytvoří RSA podpis (výchozí SHA-256); vrací base64. |
| `k.verify` | `data, signature, key[, alg]` | Ověří RSA podpis; vrací true/false. |
## Databáze

| Funkce | Parametry | Popis |
|---|---|---|
| `k.connect_db` | `dsn` | Otevře databázové připojení (schéma DSN: sqlite://, mysql://, postgres://, sqlserver://) a vrátí handle. Podporované ovladače: SQLite (vestavěný), MySQL, PostgreSQL, SQL Server. |
| `k.connect_sqlite` | `path` | Otevře SQLite databázový soubor; vrátí handle použitelný s k.sql/k.db_*. |
| `k.db_delete` | `handle, table, whereTable` | Smaže řádky odpovídající where tabulce. |
| `k.db_insert` | `handle, table, keyvalsTable` | Vloží řádek; vrací {last_insert_id, rows_affected}. |
| `k.db_kill_table` | `handle, table, where` | Smaže řádky odpovídající where tabulce. |
| `k.db_proc` | `handle, name, ...params` | Spustí uloženou proceduru. |
| `k.db_select` | `handle, table, fieldsTable, whereTable, order` | Query builder vracející {columns, rows}. |
| `k.db_update` | `handle, table, keyvalsTable, whereTable` | Aktualizuje řádky odpovídající where tabulce. |
| `k.disconnect_db` | `[handle]` | Zavře databázový handle. Bez argumentu zavře všechny handly včetně připojení předregistrovaných přes --db. |
| `k.disconnect_sqlite` | `[handle]` | Zavře SQLite připojení (nebo všechna). |
| `k.rows` | `result` | Vrátí iterátor přes řádky výsledku dotazu. |
| `k.sql` | `handle, query, ...params` | Spustí libovolný SQL; vrací řádky nebo {rows_affected}. |
| `k.tx_begin` | `handle` | Zahájí transakci na připojení. |
| `k.tx_commit` | `handle` | Potvrdí aktivní transakci. |
| `k.tx_rollback` | `handle` | Vrátí zpět (rollback) aktivní transakci. |
## Ladění a debug

| Funkce | Parametry | Popis |
|---|---|---|
| `k.debug` | `—` | Pomocné funkce pro runtime introspekci: stack/locals/trace. |
| `k.debug.locals` | `[level]` | Vrátí tabulku lokálních jmen → hodnot pro danou úroveň rámce (výchozí 1). |
| `k.debug.stack` | `—` | Vrátí tabulku aktuálních volacích rámců, každý s level, name, source, line a locals. |
| `k.debug.trace` | `[msg]` | Zaloguje kotvu trasování na straně skriptu, když je povoleno verbose trasování. |
## E-mail (SMTP / POP3)

| Funkce | Parametry | Popis |
|---|---|---|
| `k.pop3_connect` | `—` | Připojí se k POP3 serveru; vrací handle. |
| `k.pop3_dele` | `handle, index` | Označí zprávu ke smazání. |
| `k.pop3_list` | `handle` | Vrátí tabulku souhrnů zpráv {id, size}. |
| `k.pop3_noop` | `handle` | Udržuje POP3 připojení naživu. |
| `k.pop3_quit` | `handle` | Odešle QUIT a zavře POP3 připojení. |
| `k.pop3_retr` | `handle, index` | Získá zprávu podle indexu. |
| `k.pop3_stat` | `handle` | Vrátí {count, size} poštovní schránky. |
| `k.smtp_connect` | `—` | Připojí se k SMTP serveru; vrací handle. |
| `k.smtp_disconnect` | `[handle]` | Zavře SMTP připojení (nebo všechna). |
| `k.smtp_send` | `handle, {from,to,subject,body,attachments}` | Odešle e-mail přes připojený SMTP server. |
## Práce se soubory

| Funkce | Parametry | Popis |
|---|---|---|
| `k.file_close` | `handle` | Zavře otevřený souborový handle. |
| `k.file_copy` | `src, dst` | Zkopíruje soubor se zachováním oprávnění. |
| `k.file_delete` | `path` | Smaže soubor. |
| `k.file_exists` | `path` | Zjistí, zda cesta existuje. |
| `k.file_info` | `path` | Vrátí {name, size, is_dir, modified} pro cestu. |
| `k.file_list` | `dir` | Vypíše adresář jako 1-založenou seřazenou tabulku názvů. |
| `k.file_load` | `path` | Přečte celý soubor jako řetězec (async; max 16 MiB). |
| `k.file_mkdir` | `path[, parents]` | Vytvoří adresář; parents=true vytvoří i mezilehlé adresáře. |
| `k.file_move` | `src, dst` | Přesune/přejmenuje soubor. |
| `k.file_open` | `path[, mode]` | Otevře soubor; mode je r, r+, w, w+, a nebo a+ (výchozí r). Vrací handle. |
| `k.file_read` | `handle[, count]` | Přečte celý soubor nebo count bajtů; prázdný řetězec na konci souboru (EOF). |
| `k.file_read_line` | `handle` | Přečte jeden řádek (koncový nový řádek je odstraněn); nil na konci souboru. |
| `k.file_save` | `path, data` | Zapíše soubor atomicky (async; dočasný soubor + přejmenování). |
| `k.file_write` | `handle, data` | Zapíše data do otevřeného souboru. |
| `k.zip_add` | `zipPath, entries` | Zapíše zip archiv z položek {name=content}. |
| `k.zip_extract` | `zipPath, dir` | Rozbalí zip archiv do adresáře; vrací počet souborů. |
| `k.zip_list` | `zipPath` | Vypíše názvy členů zip archivu. |
## Tok programu

| Funkce | Parametry | Popis |
|---|---|---|
| `k.assign` | `target, kind, value` | Nastaví globální proměnnou nebo hodnotu ovládacího prvku s typovou konverzí. target: řetězec (jméno globálu) nebo tabulka {form=, ctrl=}. kind: "numeric"|"string"|"boolean"|"date". Vrací převedenou hodnotu. |
| `k.bell` | `—` | Přehrává systémové pípnutí přes WebAudio. |
| `k.clipboard_get` | `—` | Přečte text ze schránky prohlížeče. |
| `k.clipboard_set` | `text` | Zapíše text do schránky prohlížeče. |
| `k.error` | `msg` | Vyvolá záměrnou Lua chybu. |
| `k.exec` | `name, ...` | Spustí dříve uloženou funkci (přes k.set) asynchronně se zadanými argumenty. Vrací výsledek(y) funkce. |
| `k.http_request` | `optsTable` | Provede HTTP požadavek asynchronně; vrací {status, headers, body}. Pozastaví skript, dokud nepřijde odpověď. |
| `k.locale` | `—` | Vrátí locale relace (výchozí "en-US"). |
| `k.msgbox` | `opts` | Zobrazí dialog s hlášením a vrátí hodnotu kliknutého tlačítka. Legacy forma: `k.msgbox(text[, kind])`, kde kind je `info`/`warn`/`error`/`ok-cancel`/`yes-no` (vrací `"ok"`, `"cancel"`, `"yes"`, `"no"`). Bohatá forma přijímá jedinou tabulku options: `type` nastaví levý barevný proužek (`info` modrý, `warning` jantarový, `danger` červený); `buttons` je seznam dvojic `{label, value}` (hodnoty si zachovávají typ: číslo, boolean nebo řetězec), tabulek `{label=…, value=…}` nebo holých řetězců (label = value). Pokud není zadáno, přidá se jediné tlačítko `OK` vracející `"ok"`. |
| `k.net_ok` | `timeout_ms` | Zjistí dosažitelnost internetu přes TCP připojení. |
| `k.on_error` | `fn` | Zaregistruje (nebo smaže, s nil) Kalipso chybový hook. Když je nastaven ERRORCODE — selháním vazby nebo skutečnou Lua chybou — zavolá se fn s (ERRORCODE, ERRORMSG), aby skript mohl zobrazit chybu a pokračovat. Vazby, které skončí chybou, vrací nil; větvi podle ERRORCODE. |
| `k.param_get` | `key` | Přečte uložený parametr aplikace (řetězec; "" pokud není nastaven). |
| `k.param_set` | `key, value` | Uloží parametr aplikace (řetězec) do souboru na straně aplikace. |
| `k.pick_file` | `[opts]` | Otevře dialog výběru souboru v prohlížeči. mode="open" (výchozí): vybere existující soubory a vrátí tabulku {{name, size, type, data}, …} s daty v base64. mode="save": zobrazí dialog uložení s názvem souboru a vrátí {path, name}. mode="download": spustí stažení dat v base64 a vrátí {path, name}. Vráti nil při zrušení. Pozastaví skript, dokud se dialog nedokončí. |
| `k.ping` | `host, timeout_ms` | TCP-založená sonda latence vracející ms, nebo nil, když je cíl nedostupný. |
| `k.popup` | `items[, opts]` | Zobrazí víceúrovňové menu ve stylu vyskakovacího okna (centrovaný modal, vysouvací podnabídky) a vrátí hodnotu vybrané položky se zachovaným typem, nebo nil při zavření (Esc / kliknutí mimo). Položky jsou listy (label, value) nebo větve ({label, items={…}}, které jen otevřou podnabídku, až 8 úrovní). Listy přijímají dvojice {label, value}, tabulky {label=…, value=…} nebo holé řetězce (label = value); hodnoty se vrací typované (číslo, boolean, řetězec, tabulka). Alternativní list forma (bez titulku): k.popup{ {"Open", "open"}, {"Quit"} }. |
| `k.print` | `...` | Vypíše hodnoty do logu aplikace (oddělené tabulátory, jako Lua print). |
| `k.quit` | `—` | Požádá o čisté ukončení aplikace. |
| `k.screen_size` | `—` | Vrátí rozměry viewportu jako {width, height}. |
| `k.set` | `name, fn` | Uloží funkci do registru akcí pro pozdější spuštění přes k.exec. |
| `k.sleep` | `ms` | Pozastaví skript na ms milisekund. |
| `k.status_close` | `—` | Skryje stavovou lištu. |
| `k.status_show` | `text` | Zobrazí lištu busy/status s daným textem. |
| `k.timer_start` | `id, ms[, repeats]` | Spustí session timer; každých ms spustí Lua funkci pojmenovanou id (repeats-krát, pokud je zadáno, jinak stále dokola). |
| `k.timer_stop` | `id` | Zastaví běžící session timer. |
| `k.yield` | `—` | Odevzdá řízení aktuálního coroutine, čímž umožní běh ostatním coroutines. |
## Formáty dat (CSV, INI, XML, YAML)

| Funkce | Parametry | Popis |
|---|---|---|
| `k.csv_load` | `path[, opts]` | Přečte a zpracuje (parsuje) CSV soubor. |
| `k.csv_parse` | `text[, opts]` | Parsuje CSV (opts {header, sep, quote}). |
| `k.csv_save` | `path, data[, opts]` | Zapíše CSV soubor atomicky. |
| `k.csv_string` | `data[, opts]` | Serializuje CSV z tabulky. |
| `k.ini_load` | `path` | Přečte a zpracuje (parsuje) INI soubor. |
| `k.ini_parse` | `text` | Parsuje INI do {section={key=value}, _root={…}}. |
| `k.ini_read` | `path, section, key` | Přečte jeden INI klíč (kompatibilita s Kalipso). |
| `k.ini_save` | `path, data` | Zapíše INI soubor atomicky. |
| `k.ini_string` | `data` | Serializuje INI z tabulky. |
| `k.ini_write` | `path, section, key, value` | Zapíše jeden INI klíč (kompatibilita s Kalipso). |
| `k.xml_load` | `path` | Přečte XML soubor do tvaru element-tabulky {_name, _attrs, _children, _text}. |
| `k.xml_save` | `path, table` | Zapíše element-tabulku jako XML. |
| `k.yaml_load` | `path` | Přečte a zpracuje (parsuje) YAML soubor. |
| `k.yaml_parse` | `text` | Parsuje YAML; více-dokumentový vstup vrací seznam tabulek. |
| `k.yaml_save` | `path, data` | Zapíše YAML soubor atomicky. |
| `k.yaml_string` | `data` | Serializuje hodnotu jako YAML. |
## Formuláře

| Funkce | Parametry | Popis |
|---|---|---|
| `k.form` | `—` | Deklarace formulářů: k.form.new/show/close/… |
| `k.form.clear` | `name` | Vymaže hodnoty ovládacích prvků formuláře. |
| `k.form.close` | `[name]` | Zavře horní formulář, nebo pojmenovaný formulář. |
| `k.form.new` | `name, optsTable` | Deklaruje formulář. opts: {title, layout=vertical|grid, align=left|center|right, gap=n px, cells}. Grid buňky: {id={width 1–12, bg, border={width, color}, align}} nebo uspořádané pole {id, …}; prvky přiřadíte přes opt cell="id" a zarovnání přepíšete přes align. |
| `k.form.on` | `přetížení: `(form, ctrl, event, fn)` / `(name, event, fn)` / `(name, "on_idle", ms, fn)`` | Zaregistruje obsluhu události. 4-argumentová forma: obsluha prvku (form, ctrl, event, fn) pro události prvků (např. "onclick"). 3-argumentová forma: obsluha na úrovni formuláře (name, event, fn) pro události jako open_form, after_open_form, close_form, key_pressed (použije se jako fallback, když neexistuje obsluha prvku). 4-argumentová forma s číslem: k.form.on(name, "on_idle", ms, fn) nastavuje periodický idle callback (interval ms, výchozí 1000) — idle události dostává jen nejsvrchnější formulář. |
| `k.form.refresh` | `name` | Znovu vykreslí a odešle formulář do prohlížeče. |
| `k.form.return_to` | `name` | Zavře všechny formuláře nad name (vrátí se k němu). |
| `k.form.show` | `name, [options]` | Zobrazí formulář a pozastaví skript, dokud se nezavře. Ve výchozím nastavení formulář vyplní jeviště (normal). S options.modal=true se zobrazí jako centrovaný modální překryv (overlay) s konfigurovatelnou mezerou od okrajů obrazovky. |
| `k.get_property` | `form, prop` | Přečte vlastnost formuláře; vrací nil, když vlastnost nebo formulář není nastaven. |
| `k.set_property` | `form, prop, value` | Nastaví vlastnost formuláře a znovu jej vykreslí. Podporuje title, align, gap a dynamické stylovací props: bg (barva pozadí), color (barva textu), font (CSS font-family nebo text preset h1–h6/p), font_size (číselné px, přepíše preset), style (text preset h1–h6/p). Rezervované klíče name, controls, handlers a order nelze nastavit. |
## JSON

| Funkce | Parametry | Popis |
|---|---|---|
| `k.is_null` | `value` | Zjistí, zda je hodnota sentinel K.NULL. |
| `k.json_array_item` | `root, path, index` | Vrátí prvek pole na 0-založeném indexu. |
| `k.json_count` | `root, path` | Vrátí počet prvků (délku pole nebo velikost objektu). |
| `k.json_get` | `root, path` | Prochází tečkovou/lomenou cestu přes zpracovanou hodnotu, např. "a.b[0].c". |
| `k.json_load` | `path` | Přečte a zpracuje (parsuje) JSON soubor (async; max 16 MiB). |
| `k.json_names` | `root, path` | Vrátí 1-založenou tabulku klíčů nebo indexů. |
| `k.json_parse` | `text` | Parsuje JSON text; null se mapuje na K.NULL. |
| `k.json_save` | `path, value` | Zakóduje hodnotu a zapíše ji atomicky (async). |
| `k.json_string` | `value` | Zakóduje hodnotu jako kompaktní JSON (seřazené klíče). |
## Konverze result-setů

| Funkce | Parametry | Popis |
|---|---|---|
| `k.csv_to_rows` | `csvTable` | Převede zpracovanou CSV tabulku na {columns, rows}. |
| `k.json_to_rows` | `value` | Převede JSON pole map řádků na {columns, rows}. |
| `k.rows_to_csv` | `result[, opts]` | Serializuje result set jako CSV. |
| `k.rows_to_json` | `result` | Extrahuje pole řádků z result setu. |
| `k.rows_to_xml` | `result[, rootName[, rowName]]` | Serializuje result set jako XML. |
| `k.xml_to_rows` | `document` | Převede XML element-tabulku na {columns, rows}. |
## Server mód (sdílený stav, TCP, WebSocket)

| Funkce | Parametry | Popis |
|---|---|---|
| `k.shared` | `—` | Sdílený stav napříč workers: k.shared.set/get/del/keys/incr. |
| `k.shared.del` | `key` | Smaže klíč ze sdíleného stavu. |
| `k.shared.get` | `key` | Získá hodnotu ze sdíleného stavu; prázdný řetězec, pokud chybí. |
| `k.shared.incr` | `key[, delta]` | Zvýší číselný klíč o delta (výchozí 1); vrací novou hodnotu. |
| `k.shared.keys` | `[pattern]` | Vrátí všechny klíče odpovídající vzoru (prefix, * = všechny). |
| `k.shared.set` | `key, value` | Uloží řetězcovou hodnotu do sdíleného stavu. |
| `k.tcp` | `—` | TCP operace: k.tcp.send/close/accept. |
| `k.tcp.accept` | `(bez parametrů)` | Čeká na příchozí TCP připojení a vrací {id}. Připojení pak lze použít s k.tcp.send a k.tcp.close. (Pouze v serve režimu.) |
| `k.tcp.close` | `client_id` | Zavře TCP připojení. |
| `k.tcp.send` | `client_id, data` | Odešle data konkrétnímu TCP klientovi. |
| `k.ws` | `—` | WebSocket operace: k.ws.broadcast/send/close. |
| `k.ws.broadcast` | `message` | Rozešle textovou zprávu všem připojeným WebSocket klientům. |
| `k.ws.close` | `client_id` | Zavře WebSocket připojení. |
| `k.ws.send` | `client_id, message` | Odešle textovou zprávu konkrétnímu WebSocket klientovi. |
## XML

| Funkce | Parametry | Popis |
|---|---|---|
| `k.xml_attr` | `doc, path, name` | Vrátí hodnotu atributu na cestě. |
| `k.xml_attrs` | `doc, path` | Vrátí všechny atributy prvku na cestě jako tabulku. |
| `k.xml_child` | `doc, path` | Vrátí podřízený prvek na cestě (např. "book/author"). |
| `k.xml_child_list` | `doc, path` | Vrátí tabulku podřízených prvků na cestě. |
| `k.xml_content` | `doc, path` | Vrátí textový obsah prvku na cestě. |
| `k.xml_name` | `doc, path` | Vrátí název prvku na cestě. |
| `k.xml_parse` | `text` | Parsuje XML text a vrací document handle. |
| `k.xml_root` | `doc` | Vrátí název kořenového prvku zpracovaného dokumentu. |
