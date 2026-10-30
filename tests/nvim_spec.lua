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
local play_map = vim.fn.maparg("<C-CR>", "t", false, true)
assert(play_map.buffer == 1 and type(play_map.callback) == "function", "missing collection-play passthrough")
play_map.callback()
assert(sent[#sent] == "\27[13;5u", "wrong collection-play key encoding")
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
-- Exercise the real embedded-terminal parser and the buffer-local original
-- image relay. No outer terminal, image program or music account is involved.
local graphics = require("lazymusic.graphics")
local image_buf = vim.api.nvim_create_buf(false, true)
local image_win = vim.api.nvim_open_win(image_buf, true, {relative="editor",row=1,col=1,width=60,height=16})
local image_channel = vim.api.nvim_open_term(image_buf, {})
local output, responses = {}, {}
graphics.attach(image_buf, image_channel, {
  write = function(data) output[#output+1] = data end,
  send = function(data) responses[#responses+1] = data end,
})
local id = 0x6d002a
local function image_request(body)
  vim.api.nvim_chan_send(image_channel, "\27]777;LAZYMUSIC;" .. body .. "\7")
  vim.wait(30)
end
image_request("a=q,i="..id..",f=24,s=1,v=1;AAAA")
assert(#output==1 and output[1]:find("\27_Ga=q",1,true), "image query did not cross the embedded terminal")
vim.api.nvim_exec_autocmds("TermResponse", {data={sequence="\27_Gi="..id..";OK"}})
assert(responses[1]=="\27_Gi="..id..";OK\27\\", "image response was not routed to its child")
image_request("a=T,U=1,f=100,t=d,i="..id..",p=1,c=12,r=6,q=2,m=1;AAAA")
image_request("q=2,m=0;AAAA")
assert(#output==2 and output[2]:find("m=1;AAAA",1,true) and output[2]:find("m=0;AAAA",1,true), "chunked original image relay failed")
image_request("a=f,i="..id..",r=1,f=32,t=d,s=1,v=1,q=0,m=0;AAAAAA==")
image_request("a=f,i="..id..",r=1,x=163,y=63,X=1,f=100,t=d,q=1,m=1;AAAA")
image_request("a=f,i="..id..",r=1,q=1,m=0;AAAA")
image_request("a=a,i="..id..",c=1,s=1,q=2")
assert(#output==5 and output[5]:find("a=a",1,true), "in-place record animation did not cross the relay")
for _, body in ipairs({"a=f,i="..id..",f=100;AAAA", "a=f,i="..id..",r=2,f=100;AAAA", "a=a,i="..id..",c=1,s=3", "a=f,i="..(id+7)..",r=1,f=100;AAAA"}) do
  image_request(body)
end
assert(#output==5,"animation relay accepted unrelated images or accumulating frames")
for _, body in ipairs({"a=d,d=a,i="..id, "a=d,i="..id, "a=T,U=1,f=100,t=f,i="..id..",p=1;AAAA", "a=q,i=1;AAAA"}) do
  image_request(body)
end
assert(#output==5, "unsafe or unrelated terminal controls were forwarded")
image_request("a=d,d=I,i="..id..",q=2")
image_request("a=q,i="..(id+1)..",f=24,s=1,v=1;AAAA")
image_request("a=d,d=I,i="..(id+1)..",q=2")
image_request("a=q,i="..(id+2)..",f=24,s=1,v=1;AAAA")
assert(#output==9, "switching music sources exhausted image ownership slots")
graphics.detach(image_buf)
assert(output[10]=="\27_Ga=d,d=I,i="..(id+2)..",q=2\27\\", "image cleanup affected another terminal image")
vim.api.nvim_win_close(image_win,true)
vim.api.nvim_buf_delete(image_buf,{force=true})
print("Neovim mapping, terminal input, resize, reuse and exit: PASS")
vim.cmd("qa!")
