-- Run with: nvim --headless -u NONE -i NONE -l tests/nvim_spec.lua
vim.opt.rtp:prepend(vim.fn.getcwd())
vim.o.columns = 120
vim.o.lines = 45
vim.g.mapleader = " "

local plugin = require("lazymusic")
local notices = {}
vim.notify = function(message)
  notices[#notices + 1] = message
end
vim.keymap.set("t", "<C-l>", function()
  error("global editor terminal mapping consumed a TUI key")
end)

-- A real terminal child exercises window/job lifecycle without music, accounts,
-- audio devices, network access or third-party test frameworks.
plugin.setup({ cmd = vim.fn.exepath("cat") })
local mapping = vim.fn.maparg("<leader>mm", "n", false, true)
assert(type(mapping.callback) == "function", "missing leader-mm mapping")
mapping.callback()
assert(plugin.is_open(), "mapping did not open the float")
local win = vim.api.nvim_get_current_win()
local buf = vim.api.nvim_win_get_buf(win)
local job = vim.bo[buf].channel
assert(vim.bo[buf].buftype == "terminal" and job > 0, "no terminal child")
assert(vim.fn.jobwait({ job }, 0)[1] == -1, "terminal child exited")

local original_send = vim.api.nvim_chan_send
local sent = {}
vim.api.nvim_chan_send = function(channel, bytes)
  assert(channel == job)
  sent[#sent + 1] = bytes
end
for lhs, want in pairs({ ["<Esc>"] = "\27[27u", ["<C-h>"] = "\27[104;5u", ["<C-j>"] = "\27[106;5u", ["<C-l>"] = "\27[108;5u", ["H"] = "H", ["J"] = "J", ["K"] = "K", ["L"] = "L", ["<lt>"] = "<", [">"] = ">", ["<C-u>"] = "\27[117;5u", ["<C-d>"] = "\27[100;5u", ["<C-S-,>"] = "\27[60;5u", ["<C-S-.>"] = "\27[62;5u" }) do
  local map = vim.fn.maparg(lhs, "t", false, true)
  assert(map.buffer == 1 and type(map.callback) == "function", "missing buffer-local passthrough")
  map.callback()
  assert(sent[#sent] == want, "wrong bytes passed to TUI")
end
vim.api.nvim_chan_send = original_send

local old_width = vim.api.nvim_win_get_config(win).width
vim.o.columns = 100
vim.api.nvim_exec_autocmds("VimResized", {})
assert(vim.api.nvim_win_get_config(win).width < old_width, "float did not resize")
plugin.hide()
assert(not plugin.is_open(), "hide did not close the float")
assert(vim.fn.jobwait({ job }, 0)[1] == -1, "hide terminated the TUI")
win = plugin.open()
assert(vim.api.nvim_win_get_buf(win) == buf and vim.bo[buf].channel == job, "reopen duplicated the TUI")
assert(plugin.open() == win, "repeated open duplicated the float")

vim.api.nvim_chan_send(job, "\4")
assert(vim.wait(3000, function()
  return not plugin.is_open()
end), "successful child exit did not close the float")
win = plugin.open()
local next_buf = vim.api.nvim_win_get_buf(win)
assert(next_buf ~= buf and vim.bo[next_buf].channel ~= job, "closed TUI was reused")
vim.api.nvim_chan_send(vim.bo[next_buf].channel, "\4")
assert(vim.wait(3000, function()
  return not plugin.is_open()
end))

plugin.setup({ cmd = "/does-not-exist/lazymusic" })
assert(plugin.open() == nil and not plugin.is_open(), "missing executable left an empty float")
assert(#notices == 1 and notices[1]:find("not found", 1, true), "missing startup diagnostic")
print("Neovim mapping, terminal input, resize, reuse and exit: PASS")
vim.cmd("qa!")
