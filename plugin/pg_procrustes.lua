if vim.g.loaded_pg_procrustes then
  return
end
vim.g.loaded_pg_procrustes = true

vim.api.nvim_create_user_command("PgProcrustesFormat", function()
  require("pg_procrustes").format(0)
end, { desc = "Format current buffer with pg_procrustes" })
