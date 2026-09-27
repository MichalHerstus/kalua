//go:build !wasm

// Package bindings provides native-only binding registration.
package bindings

// registerNative registers bindings that are only available in native mode
// (not in WASM): database, files, comm, SMTP, POP3, FTP, SOAP.
func registerNative(e *Env) {
	registerDB(e)
	registerFiles(e)
	registerComm(e)
	registerSMTP(e)
	registerPop3(e)
	registerFTP(e)
	registerSoap(e)
}