import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import * as vscode from "vscode";
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
  Trace,
} from "vscode-languageclient/node";

let client: LanguageClient | undefined;
let output: vscode.LogOutputChannel;

const EXE = process.platform === "win32" ? ".exe" : "";

function isExecutable(file: string): boolean {
  try {
    if (!fs.statSync(file).isFile()) {
      return false;
    }
    fs.accessSync(file, fs.constants.X_OK);
    return true;
  } catch {
    return false;
  }
}

/**
 * Finds the pg_procrustes executable. VS Code started from the Dock or the Start menu does not
 * always see the shell's PATH, so the usual install folders are searched as well.
 */
function resolveBinary(configured: string): string | undefined {
  const name = configured.trim() || "pg_procrustes";
  if (path.isAbsolute(name) || name.includes(path.sep)) {
    const p = name.startsWith("~") ? path.join(os.homedir(), name.slice(1)) : name;
    return isExecutable(p) ? p : undefined;
  }
  const home = os.homedir();
  const dirs = [
    ...(process.env.PATH ?? "").split(path.delimiter),
    "/opt/homebrew/bin",
    "/usr/local/bin",
    "/home/linuxbrew/.linuxbrew/bin",
    path.join(home, "go", "bin"),
    path.join(home, ".local", "bin"),
  ].filter(Boolean);
  for (const dir of dirs) {
    const p = path.join(dir, name + (name.endsWith(EXE) ? "" : EXE));
    if (isExecutable(p)) {
      return p;
    }
  }
  return undefined;
}

async function startClient(context: vscode.ExtensionContext): Promise<void> {
  const cfg = vscode.workspace.getConfiguration("pgProcrustes");
  const configured = cfg.get<string>("path", "pg_procrustes");
  const binary = resolveBinary(configured);
  if (!binary) {
    const pick = await vscode.window.showErrorMessage(
      `pg_procrustes: executable "${configured}" not found. Install it (brew install heptau/tap/pg-procrustes) or set pgProcrustes.path.`,
      "Open Settings",
      "Installation",
    );
    if (pick === "Open Settings") {
      void vscode.commands.executeCommand("workbench.action.openSettings", "pgProcrustes.path");
    } else if (pick === "Installation") {
      void vscode.env.openExternal(
        vscode.Uri.parse("https://github.com/heptau/pg_procrustes#installation"),
      );
    }
    return;
  }

  const serverOptions: ServerOptions = {
    command: binary,
    args: ["--lsp"],
  };
  const configPath = cfg.get<string>("configPath", "").trim();
  const clientOptions: LanguageClientOptions = {
    documentSelector: [
      { scheme: "file", language: "sql" },
      { scheme: "untitled", language: "sql" },
      { scheme: "file", language: "pgsql" },
      { scheme: "untitled", language: "pgsql" },
    ],
    initializationOptions: configPath ? { configPath } : {},
    outputChannel: output,
  };

  client = new LanguageClient("pgProcrustes", "pg_procrustes", serverOptions, clientOptions);
  const trace = cfg.get<string>("trace.server", "off");
  client.setTrace(trace === "verbose" ? Trace.Verbose : trace === "messages" ? Trace.Messages : Trace.Off);
  context.subscriptions.push({ dispose: () => void client?.stop() });
  try {
    await client.start();
  } catch (err) {
    client = undefined;
    void vscode.window.showErrorMessage(`pg_procrustes: could not start the language server (${String(err)}).`);
  }
}

async function stopClient(): Promise<void> {
  const c = client;
  client = undefined;
  if (c) {
    try {
      await c.stop();
    } catch {
      // the server was not running
    }
  }
}

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  output = vscode.window.createOutputChannel("pg_procrustes", { log: true });
  context.subscriptions.push(output);

  context.subscriptions.push(
    vscode.commands.registerCommand("pgProcrustes.restart", async () => {
      await stopClient();
      await startClient(context);
    }),
    vscode.commands.registerCommand("pgProcrustes.showOutput", () => output.show()),
    vscode.workspace.onDidChangeConfiguration(async (e) => {
      if (e.affectsConfiguration("pgProcrustes")) {
        await stopClient();
        await startClient(context);
      }
    }),
  );

  await startClient(context);
}

export async function deactivate(): Promise<void> {
  await stopClient();
}
