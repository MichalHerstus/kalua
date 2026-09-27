//go:build !wasm

// Package cli implements the wasm-bundle subcommand.
package cli

import (
	"bytes"
	"context"
	"embed"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"kalua/internal/host"
)

//go:embed wasm_assets/*
var wasmAssets embed.FS

func wasmBundleCmd(args []string) int {
	fs := flag.NewFlagSet("wasm-bundle", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		outputDir  = fs.String("o", "dist", "Output directory")
		relayFlag  = fs.Bool("relay", false, "Include relay binary in output")
		verbose    = fs.Bool("v", false, "Verbose logging")
	)

	// Parse flags in two passes so flags can appear before or after the script
	script, err := parseArgsScript(fs, args)
	if err != nil {
		return int(host.ExitUsage)
	}
	if script == "" {
		fmt.Fprintln(os.Stderr, "wasm-bundle: requires a script argument")
		return int(host.ExitUsage)
	}

	logger := host.NewLogger(*verbose)

	// Get the module root directory (where go.mod is)
	moduleRoot, err := getModuleRoot()
	if err != nil {
		logger.Errorf("cannot find module root: %v", err)
		return int(host.ExitError)
	}

	// Read the script
	scriptSrc, err := os.ReadFile(script)
	if err != nil {
		logger.Errorf("cannot read %s: %v", script, err)
		if os.IsNotExist(err) || os.IsPermission(err) {
			return int(host.ExitIOError)
		}
		return int(host.ExitError)
	}

	// Create output directory
	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		logger.Errorf("cannot create output dir: %v", err)
		return int(host.ExitError)
	}

	// Build WASM binary
	logger.Printf("Building WASM binary...")
	wasmPath := filepath.Join(*outputDir, "KALUA.wasm")
	cmd := exec.CommandContext(context.Background(), "go", "build",
		"-o", wasmPath,
		"-ldflags=-s -w",
		"./internal/wasm")
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logger.Errorf("WASM build failed: %v", err)
		return int(host.ExitError)
	}
	logger.Printf("WASM binary: %s", wasmPath)

	// Copy wasm_exec.js
	logger.Printf("Copying wasm_exec.js...")
	wasmExecSrc := getWasmExecPath()
	wasmExecDst := filepath.Join(*outputDir, "wasm_exec.js")
	if err := copyFile(wasmExecSrc, wasmExecDst); err != nil {
		logger.Errorf("cannot copy wasm_exec.js: %v", err)
		return int(host.ExitError)
	}

	// Copy relay binary if requested
	if *relayFlag {
		logger.Printf("Building relay binary...")
		relayPath := filepath.Join(*outputDir, "relay")
		cmd := exec.CommandContext(context.Background(), "go", "build",
			"-o", relayPath,
			"-ldflags=-s -w",
			"./cmd/KALUA")
		cmd.Env = append(os.Environ(), "GOOS="+getGOOS(), "GOARCH="+getGOARCH())
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			logger.Errorf("relay build failed: %v", err)
			return int(host.ExitError)
		}
		logger.Printf("Relay binary: %s", relayPath)
	}

	// Generate index.html
	logger.Printf("Generating index.html...")
	indexPath := filepath.Join(*outputDir, "index.html")
	if err := generateIndexHTML(indexPath, string(scriptSrc), *relayFlag, wasmAssets); err != nil {
		logger.Errorf("cannot generate index.html: %v", err)
		return int(host.ExitError)
	}

	logger.Printf("Bundle created in %s/", *outputDir)
	return int(host.ExitOK)
}

func getModuleRoot() (string, error) {
	cmd := exec.Command("go", "env", "GOMOD")
	cmd.Dir = "/Users/michalherstus/dev/kalua" // fallback
	out, err := cmd.Output()
	if err != nil {
		// Try to find go.mod by walking up from current dir
		dir, err := os.Getwd()
		if err != nil {
			return "", err
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		return "", fmt.Errorf("go.mod not found")
	}
	modPath := strings.TrimSpace(string(out))
	if modPath == "" {
		return "", fmt.Errorf("GOMOD not set")
	}
	return filepath.Dir(modPath), nil
}

func getGoRoot() string {
	cmd := exec.Command("go", "env", "GOROOT")
	out, err := cmd.Output()
	if err != nil {
		return "/usr/local/go"
	}
	return strings.TrimSpace(string(out))
}

func getWasmExecPath() string {
	goRoot := getGoRoot()
	// Try the new location first (Go 1.21+)
	newPath := filepath.Join(goRoot, "lib", "wasm", "wasm_exec.js")
	if _, err := os.Stat(newPath); err == nil {
		return newPath
	}
	// Fallback to old location
	oldPath := filepath.Join(goRoot, "misc", "wasm", "wasm_exec.js")
	return oldPath
}

func getGOOS() string {
	cmd := exec.Command("go", "env", "GOOS")
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

func getGOARCH() string {
	cmd := exec.Command("go", "env", "GOARCH")
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func generateIndexHTML(outputPath, scriptSrc string, includeRelay bool, assets embed.FS) error {
	// Read embedded assets
	kaluaCSS, _ := assets.ReadFile("wasm_assets/kalua.css")
	tabulatorCSS, _ := assets.ReadFile("wasm_assets/tabulator.min.css")
	tabulatorSimpleCSS, _ := assets.ReadFile("wasm_assets/tabulator_simple.min.css")
	flatpickrCSS, _ := assets.ReadFile("wasm_assets/flatpickr.min.css")
	tabulatorJS, _ := assets.ReadFile("wasm_assets/tabulator.min.js")
	chartJS, _ := assets.ReadFile("wasm_assets/chart.umd.js")
	flatpickrJS, _ := assets.ReadFile("wasm_assets/flatpickr.min.js")
	appJS, _ := assets.ReadFile("wasm_assets/app.js")

	// Escape script for embedding
	scriptEscaped := template.JSEscapeString(scriptSrc)

	tmpl := template.Must(template.New("index").Parse(indexHTMLTemplate))

	var relayJS string
	if includeRelay {
		relayJS = `
		// Auto-connect to relay if available
		if (window.kaluaRelayURL) {
			window.kaluaRegisterCallbacks(function() {
				if (window.kaluaStartApp) {
					window.kaluaStartApp(scriptSrc).then(function(result) {
						if (result.ok) {
							window.kaluaPostMessage(JSON.stringify({type: 'relay_connect', url: window.kaluaRelayURL}));
						}
					});
				}
			});
		}
		`
	}

	data := struct {
		ScriptSrc      string
		KaluaCSS       string
		TabulatorCSS   string
		TabulatorSimpleCSS string
		FlatpickrCSS   string
		TabulatorJS    string
		ChartJS        string
		FlatpickrJS    string
		AppJS          string
		RelayJS        string
	}{
		ScriptSrc:         scriptEscaped,
		KaluaCSS:          string(kaluaCSS),
		TabulatorCSS:      string(tabulatorCSS),
		TabulatorSimpleCSS: string(tabulatorSimpleCSS),
		FlatpickrCSS:      string(flatpickrCSS),
		TabulatorJS:       string(tabulatorJS),
		ChartJS:           string(chartJS),
		FlatpickrJS:       string(flatpickrJS),
		AppJS:             string(appJS),
		RelayJS:           relayJS,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	return os.WriteFile(outputPath, buf.Bytes(), 0o644)
}

const indexHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>KALUA WASM App</title>
    <style>{{.KaluaCSS}}</style>
    <style>{{.TabulatorCSS}}</style>
    <style>{{.TabulatorSimpleCSS}}</style>
    <style>{{.FlatpickrCSS}}</style>
</head>
<body>
    <div id="app">
        <div id="stage"></div>
        <div id="modals"></div>
        <div id="status-bar" class="hidden"></div>
    </div>
    <script>
        // Embedded script source
        const scriptSrc = {{.ScriptSrc}};
    </script>
    <script>{{.TabulatorJS}}</script>
    <script>{{.ChartJS}}</script>
    <script>{{.FlatpickrJS}}</script>
    <script>{{.AppJS}}</script>
    <script>
        // Initialize WASM
        const go = new Go();
        WebAssembly.instantiateStreaming(fetch('KALUA.wasm'), go.importObject)
            .then(function(result) {
                go.run(result.instance);
                // Start the app after WASM is ready
                if (window.kaluaStartApp) {
                    window.kaluaStartApp(scriptSrc).then(function(result) {
                        if (result.ok) {
                            console.log('[KALUA] App started');
                        } else {
                            console.error('[KALUA] Failed to start app:', result.error);
                        }
                    });
                }
            })
            .catch(function(err) {
                console.error('[KALUA] WASM load failed:', err);
            });
        {{.RelayJS}}
    </script>
</body>
</html>`