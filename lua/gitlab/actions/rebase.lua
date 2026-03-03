local u = require("gitlab.utils")

local M = {}

---@class RebaseOpts
---@field skip_ci boolean?

local can_rebase = function()
  local git = require("gitlab.git")
  -- Check if there are local changes (couldn't run `git pull` after rebasing)
  local has_clean_tree, err = git.has_clean_tree()
  if not has_clean_tree then
    u.notify("Cannot rebase when there are changed files", vim.log.levels.ERROR)
    return false
  elseif err ~= nil then
    u.notify("Error while inspecting working tree", vim.log.levels.ERROR)
    return false
  end
  return true
end

---@param opts RebaseOpts
M.rebase = function(opts)
  if not can_rebase() then
    return
  end

  -- TODO: check that MR needs rebasing (requires https://github.com/harrisoncramer/gitlab.nvim/pull/532)

  local state = require("gitlab.state")
  local rebase_body = { skip_ci = state.settings.rebase_mr.skip_ci }
  if opts and opts.skip_ci ~= nil then
    rebase_body.skip_ci = opts.skip_ci
  end

  M.confirm_rebase(rebase_body)
end

---@param merge_body RebaseOpts
M.confirm_rebase = function(merge_body)
  local job = require("gitlab.job")
  job.run_job("/mr/rebase", "POST", merge_body, function(data)
    u.notify(data.message, vim.log.levels.INFO)
    u.notify("Implement pulling", vim.log.levels.INFO)
    u.notify("Implement updating the reviewer", vim.log.levels.INFO)
  end)
end

return M
