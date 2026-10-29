local M = {}

local defaults = {
	cmd = "lazymusic",
	args = {},
	width = 0.84,
	height = 0.82,
	border = "rounded",
	keymap = "<leader>mm",
}
local options = vim.deepcopy(defaults)
local state = {}
local configured = false

local function valid_win(win)
	return win and vim.api.nvim_win_is_valid(win)
end

local function geometry()
	local columns = vim.o.columns - 2
	local lines = vim.o.lines - vim.o.cmdheight - 2
	if columns < 60 or lines < 16 then
		return nil
	end
	local function size(value, available, minimum)
		local wanted = value <= 1 and math.floor(available * value) or math.floor(value)
		return math.min(available, math.max(minimum, wanted))
	end
	local width = size(options.width, columns, 60)
	local height = size(options.height, lines, 16)
	return {
		relative = "editor",
		style = "minimal",
		border = options.border,
		title = " lazymusic ",
		title_pos = "center",
		width = width,
		height = height,
		col = math.floor((vim.o.columns - width - 2) / 2),
		row = math.floor((vim.o.lines - vim.o.cmdheight - height - 2) / 2),
	}
end

local function running()
	return state.job and vim.fn.jobwait({ state.job }, 0)[1] == -1
end

local function command()
	local executable = vim.fn.expand(options.cmd)
	if vim.fn.executable(executable) == 0 and options.cmd == "lazymusic" then
		executable = vim.fn.expand("~/.local/bin/lazymusic")
	end
	if vim.fn.executable(executable) == 0 then
		vim.notify("lazymusic executable not found; run make install or set opts.cmd", vim.log.levels.ERROR)
		return nil
	end
	local argv = { executable }
	vim.list_extend(argv, options.args)
	return argv
end

local function passthrough(buf, job)
	-- Buffer-local mappings keep editor terminal-navigation maps from consuming
	-- the TUI's focus, paging, resize, reset and Zen shortcuts. Esc belongs to the TUI.
	-- Printable keys (including ? and /) are sent as text by the terminal.
	-- The TUI resolves Global/panel actions; navigation keys have no timing logic.
	local keys = {
		["<Esc>"] = "\27[27u",
		["<C-h>"] = "\27[104;5u",
		["<C-j>"] = "\27[106;5u",
		["<C-k>"] = "\27[107;5u",
		["<C-u>"] = "\27[117;5u",
		["<C-d>"] = "\27[100;5u",
		["H"] = "H", ["J"] = "J", ["K"] = "K", ["L"] = "L",
		["<lt>"] = "<", [">"] = ">",
		["<C-S-,>"] = "\27[60;5u", ["<C-S-.>"] = "\27[62;5u",
		["<C-<>"] = "\27[60;5u", ["<C->>"] = "\27[62;5u",
		["<C-l>"] = "\27[108;5u",
		["<C-=>"] = "\27[61;5u",
		["<C-+>"] = "\27[43;5u",
		["<C-->"] = "\27[45;5u",
	}
	for lhs, bytes in pairs(keys) do
		vim.keymap.set("t", lhs, function()
			if vim.fn.jobwait({ job }, 0)[1] == -1 then
				vim.api.nvim_chan_send(job, bytes)
			end
		end, { buffer = buf, silent = true, nowait = true, desc = "lazymusic input" })
	end
end

function M.is_open()
	return not not valid_win(state.win)
end

function M.hide()
	if valid_win(state.win) then
		vim.api.nvim_win_close(state.win, true)
	end
	state.win = nil
end

function M.open()
	if not configured then
		M.setup()
	end
	if valid_win(state.win) and running() then
		vim.api.nvim_set_current_win(state.win)
		vim.cmd.startinsert()
		return state.win
	end
	local config = geometry()
	if not config then
		vim.notify("lazymusic needs at least 60 x 16 cells inside its floating window", vim.log.levels.WARN)
		return nil
	end
	local argv = command()
	if not argv then
		return nil
	end
	if not running() or not state.buf or not vim.api.nvim_buf_is_valid(state.buf) then
		M.hide()
		if state.buf and vim.api.nvim_buf_is_valid(state.buf) then
			vim.api.nvim_buf_delete(state.buf, { force = true })
		end
		state = { buf = vim.api.nvim_create_buf(false, true) }
		vim.bo[state.buf].bufhidden = "hide"
		vim.bo[state.buf].swapfile = false
	end
	state.win = vim.api.nvim_open_win(state.buf, true, config)
	vim.wo[state.win].winblend = 0
	if not running() then
		local buf = state.buf
		local job_options = {
			on_exit = function(_, code)
				vim.schedule(function()
					if state.buf ~= buf then
						return
					end
					state.job = nil
					if code == 0 then
						M.hide()
						if vim.api.nvim_buf_is_valid(buf) then
							vim.api.nvim_buf_delete(buf, { force = true })
						end
						state.buf = nil
					else
						vim.notify(
							"lazymusic exited with code " .. code .. "; see terminal output",
							vim.log.levels.ERROR
						)
					end
				end)
			end,
		}
		if vim.fn.has("nvim-0.11") == 1 then
			job_options.term = true
			state.job = vim.fn.jobstart(argv, job_options)
		else
			state.job = vim.fn.termopen(argv, job_options)
		end
		if state.job <= 0 then
			M.hide()
			vim.api.nvim_buf_delete(buf, { force = true })
			state = {}
			vim.notify("Could not start lazymusic", vim.log.levels.ERROR)
			return nil
		end
		passthrough(buf, state.job)
		vim.bo[buf].filetype = "lazymusic"
	end
	vim.cmd.startinsert()
	return state.win
end

function M.setup(opts)
	options = vim.tbl_deep_extend("force", defaults, opts or {})
	configured = true
	if options.keymap then
		vim.keymap.set("n", options.keymap, M.open, { silent = true, desc = "Open lazymusic" })
	end
	vim.api.nvim_create_user_command("LazyMusic", M.open, { desc = "Open lazymusic", force = true })
	vim.api.nvim_create_user_command(
		"LazyMusicHide",
		M.hide,
		{ desc = "Hide lazymusic without stopping its TUI", force = true }
	)
	local group = vim.api.nvim_create_augroup("lazymusic", { clear = true })
	vim.api.nvim_create_autocmd("VimResized", {
		group = group,
		callback = function()
			local config = geometry()
			if valid_win(state.win) and config then
				vim.api.nvim_win_set_config(state.win, config)
			end
		end,
	})
end

return M
