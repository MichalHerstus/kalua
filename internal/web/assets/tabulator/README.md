# Tabulator Assets

Download Tabulator v6.2+ from https://github.com/olifolkerd/tabulator/releases/tag/6.2

Required files (place in this directory):
- tabulator.min.js
- tabulator.min.css
- themes/simple/simple.min.css

## Quick Download (run in this directory):

```bash
# Download Tabulator v6.2
wget https://github.com/olifolkerd/tabulator/releases/download/v6.2/tabulator.min.js
wget https://github.com/olifolkerd/tabulator/releases/download/v6.2/tabulator.min.css
wget https://github.com/olifolkerd/tabulator/releases/download/v6.2/themes/simple/simple.min.css -P themes/simple/

# Or use npm if preferred:
# npm install tabulator-tables@6.2
# cp node_modules/tabulator-tables/dist/js/tabulator.min.js .
# cp node_modules/tabulator-tables/dist/css/tabulator.min.css .
# cp node_modules/tabulator-tables/dist/css/themes/simple.min.css themes/simple/
```

## Files to embed via go:embed:
- tabulator.min.js
- tabulator.min.css
- themes/simple/simple.min.css

After downloading, rebuild: `go build -o KALUA ./cmd/KALUA`