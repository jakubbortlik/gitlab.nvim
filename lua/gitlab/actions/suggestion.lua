--- This module is responsible for previewing changes suggested in comments.
--- The data required to make the API calls are drawn from the discussion nodes.

local common = require("gitlab.actions.common")
local diffview_lib = require("diffview.lib")
local git = require("gitlab.git")
local keymaps = require("gitlab.state").settings.keymaps
local List = require("gitlab.utils.list")
local u = require("gitlab.utils")

local M = {}

local set_keymaps = function(bufnr)
  if keymaps.suggestion_preview.quit then
    vim.keymap.set("n", keymaps.suggestion_preview.quit, function()
      vim.cmd.tabclose()
    end, { buffer = bufnr, desc = "Close preview tab", nowait = keymaps.suggestion_preview.quit_nowait })
  end
end

M.show_preview = function(opts)
  local note_lines = common.get_note_lines(opts.tree)
  local root_node = common.get_root_node(opts.tree, opts.node)
  local suggestions = M.get_suggestions(note_lines)
  if #suggestions == 0 then
    u.notify("Note doesn't contain any suggestion.", vim.log.levels.WARN)
    return
  end

  if opts.node.is_draft then
    -- We're probably reviewing a draft suggestion in order to modify/test it, so we probably want
    -- the suggestion to be applied to the local version of the file, so that we get LSP
    -- functionality on it, we can run tests etc.
    -- How should multiple suggestions be dealt with in that case? Applying and discarding the
    -- patches? Maybe the local file should be nomodifiable and only the note text should be
    -- editable and some CursorHold, CursorMoved, CursorMovedI, InsertCharPre autocommands should be
    -- used to apply the suggestions on the local file.
    u.notify("Previewing a draft suggestion, showing diff against current HEAD.")
    root_node.head_sha = "HEAD"
  end

  if not git.revision_exists(root_node.head_sha) then
    u.notify(string.format("Revision %s for which the comment was made does not exist", root_node.head_sha), vim.log.levels.WARN)
    return
  end

  local original_head_text = git.get_file_revision({ file_name = opts.node.file_name, revision = root_node.head_sha })
  local head_text = git.get_file_revision({ file_name = opts.node.file_name, revision = "HEAD" })

  -- The the original head_sha doesn't contain the file, the branch was possibly rebased, and the
  -- original head_sha could not been found. In that case `git.get_file_revision` should have logged
  -- an error so we just return.
  if original_head_text == nil then
    u.notify(
      string.format("File %s doesn't contain any text in revision %s for which the comment was made", opts.node.file_name, root_node.head_sha),
      vim.log.levels.WARN
    )
    return
  end

  local view = diffview_lib.get_current_view()
  if view == nil then
    u.notify("Could not find Diffview view", vim.log.levels.ERROR)
    return
  end

  local files = view.panel:ordered_file_list()
  local file_name = List.new(files):find(function(file)
    return file.path == opts.node.file_name
  end).path

  if file_name == nil then
    u.notify("File %s not found.", file_name)
    return
  end

  for _, suggestion in ipairs(suggestions) do
    -- -- If the HEAD version is different from the opts.node.head_sha version, use something like
    -- -- this instead:
    -- local head_buf = vim.api.nvim_create_buf(false, true)
    -- vim.api.nvim_buf_set_lines(head_buf, 0, -1, false, lines)
    -- local tmp_path = vim.fn.tempname()
    -- vim.api.nvim_buf_set_name(h, tmp_path .. '/' .. file_name)
    -- vim.cmd.filetype("detect")

    vim.api.nvim_cmd({ cmd = "tabnew", args = {file_name} }, {})
    local head_buf = vim.api.nvim_get_current_buf()
    local buf_filetype = vim.api.nvim_get_option_value('filetype', {buf = head_buf})
    vim.api.nvim_set_current_buf(head_buf)

    vim.bo.buftype = 'nofile'
    vim.bo.bufhidden = 'wipe'
    vim.bo.filetype = buf_filetype

    local suggestion_buf = vim.api.nvim_create_buf(true, true)
    vim.cmd("vsplit")
    vim.api.nvim_set_current_buf(suggestion_buf)
    vim.api.nvim_buf_set_lines(suggestion_buf, 0, -1, false, lines)
    vim.bo.buftype = 'nofile'
    vim.bo.bufhidden = 'wipe'
    vim.bo.filetype = buf_filetype

    set_keymaps(suggestion_buf)

    local end_line = (opts.node.new_line or opts.node.range["end"].new_line) + suggestion.end_line
    local start_line = end_line - suggestion.start_line - 1
    vim.api.nvim_buf_set_lines(suggestion_buf, start_line, end_line, false, suggestion.lines)
    vim.bo.modifiable = false
    vim.cmd("windo diffthis")
  end
end

M.get_suggestions = function(note_lines)
  local suggestions = {}
  local in_suggestion = false
  local suggestion = {}
  local quote

  for _, line in ipairs(note_lines) do
    local start_quote = string.match(line, "^%s*(`+)suggestion:%-%d+%+%d+")
    local end_quote = string.match(line, "^%s*(`+)%s*$")

    if start_quote ~= nil and not in_suggestion then
      quote = start_quote
      in_suggestion = true
      suggestion.start_line, suggestion.end_line = string.match(line, "^%s*`+suggestion:%-(%d+)%+(%d+)")
      suggestion.lines = {}
    elseif end_quote and end_quote == quote then
      table.insert(suggestions, suggestion)
      in_suggestion = false
      suggestion = {}
    elseif in_suggestion then
      table.insert(suggestion.lines, line)
    end
  end
  return suggestions
end

return M
