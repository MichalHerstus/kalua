# Tabulator Assets

Tabulator v6.2.5 (MIT) embedded for the `k.ctrl.table` data grid.

Included files:
- `tabulator.min.js` — Tabulator v6.2.5 (UMD, defines global `Tabulator`)
- `tabulator.min.css` — base Tabulator styles
- `themes/simple/simple.min.css` — Simple theme

Downloaded from jsDelivr:
- https://cdn.jsdelivr.net/npm/tabulator-tables@6.2.5/dist/js/tabulator.min.js
- https://cdn.jsdelivr.net/npm/tabulator-tables@6.2.5/dist/css/tabulator.min.css
- https://cdn.jsdelivr.net/npm/tabulator-tables@6.2.5/dist/css/tabulator_simple.min.css

To upgrade to a newer version, re-download the three files from the matching
jsDelivr paths and rebuild: `go build -o KALUA ./cmd/KALUA` (files are embedded
via `//go:embed assets/*`).

If the library fails to load in the browser, `app.js` falls back to rendering a
plain HTML `<table>` from the `data-k-tabulator-*` attributes so tabulator=true
controls never show an empty grid.