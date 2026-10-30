-- Original-resolution image transport for the music terminal buffer only.
local M = {}
local sessions = {}
local min_id, max_id = 0x6d0001, 0x6dffff

function M.available()
  return type(vim.api.nvim_ui_send) == "function" and vim.o.termguicolors
    and #vim.api.nvim_list_uis() > 0 and not vim.env.TMUX and not vim.env.ZELLIJ
end

function M.attach(buf, job, hooks)
  hooks = hooks or {}
  local s = { ids = {}, current = nil, job = job }
  sessions[buf] = s
  local write = hooks.write or vim.api.nvim_ui_send
  local send = hooks.send or function(data) vim.api.nvim_chan_send(job, data) end
  if type(write) ~= "function" then return end
  s.write = write
  local group = vim.api.nvim_create_augroup("lazymusic.graphics." .. buf, { clear = true })
  s.group = group
  local allowed = { a=true, q=true, i=true, f=true, t=true, s=true, v=true, m=true,
    U=true, c=true, r=true, C=true, d=true, p=true, x=true, y=true, X=true }
  vim.api.nvim_create_autocmd("TermRequest", {
    group = group, buffer = buf,
    callback = function(ev)
      local sequence = ev.data and ev.data.sequence or ""
      local body = sequence:match("^\27%]777;LAZYMUSIC;(.+)$")
      if not body or #body > 4400 then return end
      body = body:gsub("\7$", ""):gsub("\27\\$", "")
      local metadata, payload = body:match("^([^;]+);?(.*)$")
      if not metadata or #metadata > 220 or payload:find("[^A-Za-z0-9+/=]") then return end
      local params = {}
      for field in metadata:gmatch("[^,]+") do
        local key, value = field:match("^([A-Za-z])=([A-Za-z0-9]+)$")
        if not key or not allowed[key] or params[key] then return end
        params[key] = value
      end
      if params.t and params.t ~= "d" then return end
      if params.f and params.f ~= "24" and params.f ~= "32" and params.f ~= "100" then return end
      if params.d and params.d ~= "I" then return end
      if params.a and not ({q=true,T=true,p=true,d=true,f=true,a=true})[params.a] then return end
      if params.a == "d" and (params.d ~= "I" or not params.i) then return end
      if (params.a == "T" or params.a == "p") and (params.U ~= "1" or params.p ~= "1") then return end
      if params.a == "f" then
        -- Only edit the existing root image, never append animation frames.
        if params.r ~= "1" or not params.i or not s.ids[tonumber(params.i)] then return end
        if params.f == "32" and (params.s ~= "1" or params.v ~= "1" or payload ~= "AAAAAA==") then return end
        if params.f and params.f ~= "100" and params.f ~= "32" then return end
      end
      if params.a == "a" then
        if params.c ~= "1" or params.s ~= "1" or not params.i or not s.ids[tonumber(params.i)] or payload ~= "" then return end
      end
      if not params.a then
        if not s.current or (params.m ~= "0" and params.m ~= "1") then return end
        for key in pairs(params) do if key ~= "m" and key ~= "q" then return end end
      end
      local id = tonumber(params.i) or s.current
      if not id or id < min_id or id > max_id then return end
      if not s.ids[id] then
        if params.a ~= "q" and params.a ~= "T" then return end
        local count = 0
        for _ in pairs(s.ids) do count = count + 1 end
        if count >= 2 then return end
        s.ids[id] = true
      end
      if params.m == "1" then s.current = id else s.current = nil end
      -- Forward only our validated direct graphics commands, never arbitrary
      -- terminal requests, file names or escape sequences from a child process.
      write("\27_G" .. body .. "\27\\")
      if params.a == "d" then s.ids[id] = nil end
    end,
  })
  vim.api.nvim_create_autocmd("TermResponse", {
    group = group,
    callback = function(ev)
      local sequence = ev.data and ev.data.sequence or ""
      local metadata = sequence:match("^\27_G([^;]+);")
      if not metadata then return end
      local id = tonumber(("," .. metadata .. ","):match(",i=(%d+),"))
      if id and s.ids[id] and #sequence < 1024 and vim.api.nvim_buf_is_valid(buf) then
        if sequence:sub(-2) ~= "\27\\" then sequence = sequence .. "\27\\" end
        send(sequence)
      end
    end,
  })
end

function M.detach(buf)
  local s = sessions[buf]
  if not s then return end
  for id in pairs(s.ids) do
    s.write(string.format("\27_Ga=d,d=I,i=%d,q=2\27\\", id))
  end
  if s.group then pcall(vim.api.nvim_del_augroup_by_id, s.group) end
  sessions[buf] = nil
end

return M
