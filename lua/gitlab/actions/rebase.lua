local u = require("gitlab.utils")

local M = {}

---@class RebaseOpts
---@field skip_ci boolean?

local can_rebase = function()
  local git = require("gitlab.git")
  -- Check if there are local changes (we wouldn't be able to run `git pull` after rebasing)
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
    local git = require("gitlab.git")
    local state = require("gitlab.state")
    local success = git.pull(state.settings.connection_settings.remote, state.INFO.source_branch, { "--rebase" })
    if success then
      u.notify(
        string.format(
          "Pulled `%s %s` successfully",
          state.settings.connection_settings.remote,
          state.INFO.source_branch
        ),
        vim.log.levels.INFO
      )
    end
  end)
end

return M
