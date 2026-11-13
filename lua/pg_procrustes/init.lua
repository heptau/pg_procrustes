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
    -- --stdin-filepath makes the config come from the buffer's directory, not from nvim's cwd
    args = vim.list_extend({ "--stdin-filepath", "$FILENAME" }, M.options.args),
    -- range formatting (visual selection): only the statements touching the lines are formatted
    range_args = function(_, ctx)
      local args = { "--stdin-filepath", "$FILENAME" }
      vim.list_extend(args, M.options.args)
      vim.list_extend(args, { "--lines", ctx.range.start[1] .. ":" .. ctx.range["end"][1] })
      return args
    end,
    stdin = true,
  }

  conform.formatters_by_ft.sql = conform.formatters_by_ft.sql or {}
  if not vim.tbl_contains(conform.formatters_by_ft.sql, "pg_procrustes") then
    table.insert(conform.formatters_by_ft.sql, "pg_procrustes")
  end
end

-- Formats the whole buffer, or - with line1/line2 - only the statements that touch those lines.
function M.format(bufnr, line1, line2)
  bufnr = bufnr or 0

  if not binary_ok() then
    vim.notify(missing_binary_message(), vim.log.levels.ERROR)
    return
  end

  local lines = vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)
  local input = table.concat(lines, "\n") .. "\n"

  local cmd = { M.options.bin }
  vim.list_extend(cmd, M.options.args)
  local name = vim.api.nvim_buf_get_name(bufnr)
  if name ~= "" then
    vim.list_extend(cmd, { "--stdin-filepath", name })
  end
  if line1 and line2 then
    vim.list_extend(cmd, { "--lines", line1 .. ":" .. line2 })
  end

  local result = vim.system(cmd, { stdin = input, text = true }):wait()

  if result.code ~= 0 then
    vim.notify("pg_procrustes: " .. vim.trim(result.stderr or "format failed"), vim.log.levels.ERROR)
    return
  end

  -- The buffer becomes exactly what the command line writes: only the line break that ends
  -- the last line is no line of its own (blank lines at the start and the end are kept).
  local out = vim.split(result.stdout, "\n", { plain = true })
  if out[#out] == "" then
    table.remove(out)
  end

  local view = vim.fn.winsaveview()
  vim.api.nvim_buf_set_lines(bufnr, 0, -1, false, out)
  vim.fn.winrestview(view)
end

return M
