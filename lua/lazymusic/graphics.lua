-- Original-resolution image transport for the music terminal buffer only.
local M = {}
local sessions = {}
local min_id, max_id = 0x6d0001, 0x6dffff
local verified_ui

-- Proof belongs to the attached host TTYs, not a song, float, buffer or job.
local function ui_key()
  local channels = {}
  for _, ui in ipairs(vim.api.nvim_list_uis()) do
    if ui.stdout_tty then channels[#channels + 1] = tostring(ui.chan) end
  end
  if #channels == 0 then return nil end
  table.sort(channels)
  return table.concat(channels, ",")
end

function M.available()
  return type(vim.api.nvim_ui_send) == "function" and vim.o.termguicolors
    and ui_key() ~= nil and not vim.env.TMUX and not vim.env.ZELLIJ
end

function M.confirmed()
  return M.available() and verified_ui ~= nil and verified_ui == ui_key()
end

function M.attach(buf, job, hooks)
  hooks = hooks or {}
  local s = { ids = {}, probes = {}, current = nil, job = job, chunks = {}, chunk_bytes = 0 }
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
        if count >= 3 then return end
        s.ids[id] = true
      end
      if params.a == "q" then s.probes[id] = true end
      local continuing = not params.a and s.current ~= nil
      if params.m == "1" then s.current = id else s.current = nil end
      -- Forward only our validated direct graphics commands, never arbitrary
      -- terminal requests, file names or escape sequences from a child process.
      local command = "\27_G" .. body .. "\27\\"
      local image_chunk = params.a == "T" or (params.a == "f" and params.f ~= "32") or continuing
      if image_chunk and (params.m == "1" or s.chunk_bytes > 0) then
        -- Keep Kitty's small validated packets, but forward a whole image in
        -- one host-UI call instead of hundreds of calls per video frame.
        if params.a == "T" or (params.a == "f" and params.f == "100") then
          s.chunks, s.chunk_bytes = {}, 0
        end
        s.chunk_bytes = s.chunk_bytes + #command
        if s.chunk_bytes > 9 * 1024 * 1024 then
          s.chunks, s.chunk_bytes, s.current = {}, 0, nil
          return
        end
        s.chunks[#s.chunks + 1] = command
        if params.m ~= "1" then
          write(table.concat(s.chunks))
          s.chunks, s.chunk_bytes = {}, 0
        end
      else
        if s.chunk_bytes > 0 then s.chunks, s.chunk_bytes = {}, 0 end
        write(command)
      end
      if params.a == "d" then s.ids[id], s.probes[id] = nil, nil end
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
        local payload = sequence:match(";(.+)$")
        if payload then payload = payload:gsub("\27\\$", "") end
        if s.probes[id] and payload == "OK" then verified_ui = ui_key() end
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
