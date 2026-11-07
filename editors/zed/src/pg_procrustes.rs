use zed_extension_api::{self as zed, serde_json, settings::LspSettings, LanguageServerId, Result};

struct PgProcrustesExtension;

impl zed::Extension for PgProcrustesExtension {
    fn new() -> Self {
        Self
    }

    fn language_server_command(
        &mut self,
        language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<zed::Command> {
        let settings = LspSettings::for_worktree(language_server_id.as_ref(), worktree).ok();
        let binary = settings.and_then(|s| s.binary);

        // `lsp.pg_procrustes.binary.path` wins, then PATH.
        let command = match binary.as_ref().and_then(|b| b.path.clone()) {
            Some(path) => path,
            None => worktree.which("pg_procrustes").ok_or_else(|| {
                "pg_procrustes not found on PATH. Install it (brew install heptau/tap/pg-procrustes, \
                 see https://github.com/heptau/pg_procrustes#installation) or set \
                 lsp.pg_procrustes.binary.path in the Zed settings."
                    .to_string()
            })?,
        };

        let args = binary
            .as_ref()
            .and_then(|b| b.arguments.clone())
            .unwrap_or_else(|| vec!["--lsp".to_string()]);
        let env = binary
            .and_then(|b| b.env)
            .map(|e| e.into_iter().collect())
            .unwrap_or_else(|| worktree.shell_env());

        Ok(zed::Command { command, args, env })
    }

    fn language_server_initialization_options(
        &mut self,
        language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<Option<serde_json::Value>> {
        // e.g. "lsp": { "pg_procrustes": { "initialization_options": { "configPath": "/x/.pg_procrustes.yaml" } } }
        Ok(LspSettings::for_worktree(language_server_id.as_ref(), worktree)
            .ok()
            .and_then(|s| s.initialization_options))
    }
}

zed::register_extension!(PgProcrustesExtension);
