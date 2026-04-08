local u = require("gitlab.utils")

local M = {}

---@class RebaseOpts
---@field skip_ci boolean? If true, a CI pipeline is not created.
---@field force boolean? If true, MR is rebased even if MR already is rebased.

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
  opts = opts or {}
  if not can_rebase() then
    return
  end

  local state = require("gitlab.state")

  if not opts.force then
    local need_rebase = vim.iter(state.MERGEABILITY):find(function(c)
      return c.identifier == "NEED_REBASE"
    end)
    if need_rebase and need_rebase.status == "SUCCESS" then
      u.notify("MR is already rebased", vim.log.levels.ERROR)
      return
    end
  end

  local rebase_body = { skip_ci = state.settings.rebase_mr.skip_ci }
  if opts.skip_ci ~= nil then
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
