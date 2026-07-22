local M = {}

local WEBSITE = "https://pg_procrustes.80.cz"

M.website = WEBSITE

M.options = {
  bin = "pg_procrustes",
  args = {},
}

local function binary_ok()
  return vim.fn.executable(M.options.bin) == 1
end

local function missing_binary_message()
  return ("pg_procrustes: binary '%s' not found on PATH. Install it from %s"):format(M.options.bin, WEBSITE)
end

function M.setup(opts)
  M.options = vim.tbl_deep_extend("force", M.options, opts or {})

  if not binary_ok() then
    vim.notify(missing_binary_message(), vim.log.levels.WARN)
    return
  end

  local ok, conform = pcall(require, "conform")
  if not ok then
    return
  end

  conform.formatters.pg_procrustes = {
    command = M.options.bin,
    args = M.options.args,
    stdin = true,
  }

  conform.formatters_by_ft.sql = conform.formatters_by_ft.sql or {}
  if not vim.tbl_contains(conform.formatters_by_ft.sql, "pg_procrustes") then
    table.insert(conform.formatters_by_ft.sql, "pg_procrustes")
  end
end

function M.format(bufnr)
  bufnr = bufnr or 0

  if not binary_ok() then
    vim.notify(missing_binary_message(), vim.log.levels.ERROR)
    return
  end

  local lines = vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)
  local input = table.concat(lines, "\n") .. "\n"

  local cmd = { M.options.bin }
  vim.list_extend(cmd, M.options.args)

  local result = vim.system(cmd, { stdin = input, text = true }):wait()

  if result.code ~= 0 then
    vim.notify("pg_procrustes: " .. vim.trim(result.stderr or "format failed"), vim.log.levels.ERROR)
    return
  end

  local view = vim.fn.winsaveview()
  vim.api.nvim_buf_set_lines(bufnr, 0, -1, false, vim.split(result.stdout, "\n", { trimempty = true }))
  vim.fn.winrestview(view)
end

return M
