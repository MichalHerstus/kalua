import { existsSync, statSync } from 'fs';
import { join } from 'path';
import * as vscode from 'vscode';
import {
  LanguageClient,
  LanguageClientOptions,
  RevealOutputChannelOn,
  ServerOptions,
  TransportKind,
} from 'vscode-languageclient/node';

let client: LanguageClient;

export function activate(context: vscode.ExtensionContext): void {
  const binary = resolveBinary(context);
  if (binary === '') {
    vscode.window.showErrorMessage(
      'KALUA binary not found. Set kalua.binaryPath or build KALUA (go build -o KALUA ./cmd/KALUA) and ensure it is on PATH.'
    );
    return;
  }

  const serverOptions: ServerOptions = {
    run: { module: binary, args: ['lsp'], transport: TransportKind.stdio },
    debug: { module: binary, args: ['lsp'], transport: TransportKind.stdio },
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ language: 'kalua' }],
    synchronize: {},
    outputChannel: vscode.window.createOutputChannel('KALUA'),
    revealOutputChannelOn: RevealOutputChannelOn.Never,
  };

  client = new LanguageClient('kalua', 'KALUA', serverOptions, clientOptions);
  client.start();

  context.subscriptions.push(
    client,
    vscode.commands.registerCommand('kalua.checkFile', () => runCli(binary, 'check')),
    vscode.commands.registerCommand('kalua.runApp', () => runCli(binary, 'run')),
    vscode.commands.registerCommand('kalua.newApp', () => newApp(binary))
  );
}

export function deactivate(): Thenable<void> | undefined {
  if (!client) {
    return undefined;
  }
  return client.stop();
}

function resolveBinary(context: vscode.ExtensionContext): string {
  const configured = vscode.workspace
    .getConfiguration('kalua')
    .get<string>('binaryPath', '')
    .trim();

  const candidates: string[] = [];
  if (configured !== '') {
    candidates.push(configured);
  } else {
    candidates.push(join(context.extensionPath, '..', '..', 'KALUA'));
    candidates.push('KALUA');
  }

  for (const c of candidates) {
    if (c.includes('/') || c.includes('\\')) {
      if (isExecutable(c)) {
        return c;
      }
    } else {
      // bare command name: resolved through PATH by the shell
      return c;
    }
  }
  return '';
}

function isExecutable(p: string): boolean {
  try {
    return existsSync(p) && statSync(p).isFile();
  } catch {
    return false;
  }
}

function activeLuaFile(): string | undefined {
  const ed = vscode.window.activeTextEditor;
  if (!ed || ed.document.languageId !== 'kalua') {
    return undefined;
  }
  return ed.document.uri.fsPath;
}

function runCli(binary: string, sub: string): void {
  const file = activeLuaFile();
  if (!file) {
    vscode.window.showWarningMessage('Open a *.lua file first.');
    return;
  }
  const terminal = vscode.window.createTerminal({ name: `KALUA: ${sub}`, cwd: vscode.workspace.workspaceFolders?.[0]?.uri.fsPath });
  terminal.show(true);
  terminal.sendText(`"${binary}" ${sub} "${file}"`, true);
}

function newApp(binary: string): void {
  vscode.window.showInputBox({ prompt: 'New KALUA app name', placeHolder: 'myapp' }).then((name) => {
    const folder = vscode.workspace.workspaceFolders?.[0];
    if (!name || !folder) {
      return;
    }
    const terminal = vscode.window.createTerminal({ name: 'KALUA: new', cwd: folder.uri.fsPath });
    terminal.show(true);
    terminal.sendText(`"${binary}" new "${name}"`, true);
  });
}