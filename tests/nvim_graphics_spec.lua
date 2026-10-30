-- Real embedded-terminal events, with a fake host TTY. No music or outer UI.
vim.opt.rtp:prepend(vim.fn.getcwd())
vim.o.columns, vim.o.lines, vim.o.termguicolors = 120, 45, true
vim.env.TMUX, vim.env.ZELLIJ, vim.env.LAZYMUSIC_IMAGE_DISABLE = nil, nil, nil
local ui = { chan = 41, stdout_tty = true }
vim.api.nvim_list_uis = function() return {ui} end
vim.api.nvim_ui_send = function() end

local graphics = require("lazymusic.graphics")
local buf = vim.api.nvim_create_buf(false, true)
local win = vim.api.nvim_open_win(buf, true, {relative="editor",row=1,col=1,width=60,height=16})
local channel = vim.api.nvim_open_term(buf, {})
local output, responses = {}, {}
graphics.attach(buf, channel, {
  write = function(data) output[#output+1] = data end,
  send = function(data) responses[#responses+1] = data end,
})
local function request(body)
  vim.api.nvim_chan_send(channel, "\27]777;LAZYMUSIC;"..body.."\7")
  vim.wait(10)
end
local function response(id, payload)
  vim.api.nvim_exec_autocmds("TermResponse", {data={sequence="\27_Gi="..id..";"..payload.."\27\\"}})
end
local probe, picture = 0x6d1100, 0x6d1101
assert(graphics.available() and not graphics.confirmed(), "unproven TTY was marked supported")
request("a=q,i="..probe..",f=24,s=1,v=1;AAAA")
request("a=T,U=1,f=100,t=d,i="..picture..",p=1,c=12,r=6,q=2,m=0;AAAA")
request("a=d,d=I,i="..picture..",q=2")
-- A cached cover completes before the query reply. Image cleanup must leave
-- query ownership intact, so this late response can still reach the child.
response(probe, "OK")
assert(#responses==1 and responses[1]:find("i="..probe,1,true), "cached artwork cleanup dropped the capability reply")
assert(graphics.confirmed(), "positive host proof was not cached")
response(probe, "ENOTSUP")
assert(graphics.confirmed(), "late error revoked already confirmed host support")
graphics.detach(buf)
assert(graphics.confirmed(), "background exit discarded proof for the same host TTY")
assert(output[#output]:find("i="..probe..",",1,true), "probe ID leaked on final detach")

-- Source changes must release both IDs. A subsequent source must still fit
-- within the same bounded ownership registry, rather than exhaust its slots.
graphics.attach(buf, channel, {write=function(data) output[#output+1]=data end,send=function()end})
for i=1,4 do
  local first = 0x6d2000+i*2
  local before = #output
  request("a=q,i="..first..",f=24,s=1,v=1;AAAA")
  request("a=T,U=1,f=100,t=d,i="..(first+1)..",p=1,c=12,r=6,q=2,m=0;AAAA")
  request("a=d,d=I,i="..(first+1)..",q=2")
  request("a=d,d=I,i="..first..",q=2")
  assert(#output==before+4, "source changes exhausted picture/query ownership slots")
end
graphics.detach(buf)
vim.api.nvim_win_close(win,true)
vim.api.nvim_buf_delete(buf,{force=true})

local plugin = require("lazymusic")
plugin.setup({cmd=vim.fn.exepath("cat")})
local launch_env
local jobstart, termopen = vim.fn.jobstart, vim.fn.termopen
vim.fn.jobstart = function(argv,opts) launch_env=opts.env;return jobstart(argv,opts) end
vim.fn.termopen = function(argv,opts) launch_env=opts.env;return termopen(argv,opts) end
local function open_and_exit()
  local opened = plugin.open()
  assert(opened, "could not reopen terminal child")
  local job = vim.bo[vim.api.nvim_win_get_buf(opened)].channel
  vim.api.nvim_chan_send(job,"\4")
  assert(vim.wait(3000,function()return not plugin.is_open()end), "terminal child did not exit")
end
for _=1,3 do
  open_and_exit()
  assert(launch_env.LAZYMUSIC_IMAGE_SUPPORTED=="1" and launch_env.LAZYMUSIC_IMAGE_BRIDGE=="osc", "same-host background reopen lost its verified image support")
end
vim.env.LAZYMUSIC_IMAGE_DISABLE="1"
open_and_exit()
assert(launch_env.LAZYMUSIC_IMAGE_DISABLE=="1" and launch_env.LAZYMUSIC_IMAGE_BRIDGE=="" and launch_env.LAZYMUSIC_IMAGE_SUPPORTED=="", "positive cache overrode explicit pixel mode")
vim.env.LAZYMUSIC_IMAGE_DISABLE=nil
ui = {chan=42,stdout_tty=true}
assert(not graphics.confirmed(), "support cache escaped to a different host UI")
open_and_exit()
assert(launch_env.LAZYMUSIC_IMAGE_SUPPORTED=="" and launch_env.LAZYMUSIC_IMAGE_BRIDGE=="osc", "new host was not left for fresh probing")
ui.stdout_tty=false
assert(not graphics.available() and not graphics.confirmed(), "GUI without a host TTY reused image support")
print("Neovim delayed image replies, background reopen and host capability cache: PASS")
vim.cmd("qa!")
