if vim.g.loaded_pg_procrustes then
  return
end
vim.g.loaded_pg_procrustes = true

vim.api.nvim_create_user_command("PgProcrustesFormat", function(opts)
  if opts.range > 0 then
    require("pg_procrustes").format(0, opts.line1, opts.line2)
  else
    require("pg_procrustes").format(0)
  end
end, { range = true, desc = "Format the current buffer (or the statements in the given range) with pg_procrustes" })
