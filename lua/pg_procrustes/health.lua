local M = {}

function M.check()
  local health = vim.health
  health.start("pg_procrustes")

  local pg = require("pg_procrustes")
  local bin = pg.options.bin

  if vim.fn.executable(bin) == 1 then
    local result = vim.system({ bin, "--version" }, { text = true }):wait()
    local version = result.code == 0 and vim.trim(result.stdout) or "unknown version"
    health.ok(("found `%s` (%s)"):format(bin, version))
  else
    health.error(("`%s` not found on PATH"):format(bin), { "Install pg_procrustes: " .. pg.website })
  end

  if pcall(require, "conform") then
    health.ok("conform.nvim detected — pg_procrustes registered as an sql formatter")
  else
    health.warn(
      "conform.nvim not found — falling back to :PgProcrustesFormat only",
      { "Install conform.nvim for format-on-save integration, or use :PgProcrustesFormat manually." }
    )
  end
end

return M
