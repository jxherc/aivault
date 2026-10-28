# ai vault

[english](README.md)

一个放规则、skill 和记忆的文件夹，你所有的 ai 编程工具都读它：claude code、codex、opencode、gemini cli、qwen code、kimi code、deepseek harness，另外还有 14 个。

在 claude code 里存一条偏好，下次打开 codex 或 qwen code，它们就知道了。skill 写一次，所有工具都能用。

## 安装

mac 或 linux：

```sh
curl -fsSL https://raw.githubusercontent.com/jxherc/aivault/main/install.sh | sh
```

windows（powershell）：

```powershell
irm https://raw.githubusercontent.com/jxherc/aivault/main/install.ps1 | iex
```

两个脚本都会下载最新的 release，校验 checksum，把 `aivault` 放进 PATH（`~/.local/bin`，windows 上是 `%LOCALAPPDATA%\Programs\aivault`），然后问你要不要接着配置。`AIVAULT_VERSION=0.2.0` 固定版本，`AIVAULT_DIR` 指定文件夹，`AIVAULT_NO_INIT=1` 跳过配置的询问。

其他方式：

| | |
| --- | --- |
| homebrew | `brew install --cask jxherc/tap/aivault` |
| scoop | `scoop bucket add jxherc https://github.com/jxherc/scoop-bucket`，然后 `scoop install aivault` |
| go | `go install github.com/jxherc/aivault/cmd/aivault@latest` |
| 手动 | 从 [releases 页面](https://github.com/jxherc/aivault/releases) 下载适合你电脑的文件，比如 `aivault-macos-apple-silicon.tar.gz` |

用脚本或手动装的，`aivault update` 会原地更新（同样会校验 checksum）。brew 和 scoop 走各自的包管理器更新，`aivault update` 会告诉你该跑什么命令。

mac 版二进制还没签名。安装脚本和 brew cask 都不会带上 quarantine 标记，所以 macos 不会拦它。用浏览器下载的文件需要先跑 `xattr -d com.apple.quarantine aivault`。

## 配置

```sh
aivault init
```

init 会问你 vault 放哪（默认 `~/ai-vault`）、要连接哪些工具、要不要在后台保持同步。然后重启这些工具，再运行：

```sh
aivault doctor
```

在脚本里用：

```sh
aivault init --yes --vault ~/ai-vault --tools claude,codex,qwen --autosync
```

> **配置完之后又装了新的 ai 工具？自己把它连上。** aivault 只接入你运行 `init` 时找到的工具，不会盯着有没有新装的。再跑一次 `aivault init`：已经连上的工具保持连接，新工具会默认勾上。`aivault doctor` 也会列出已安装但没连接的工具。

## 导入你已有的东西

如果你的工具里已经有规则、skill 或它们自己的记忆，init 会问你要不要把它们拉进 vault（脚本里用 `--import`）。之后，**每次 `aivault sync` 都会做这件事**：上次同步之后工具里新增的东西，也会进 vault。

- **规则文本**：工具全局规则文件（`~/.claude/CLAUDE.md`、`~/.codex/AGENTS.md`、`~/.qwen/QWEN.md` 等）里 ai vault 区块以外的内容，都会移进 `me.md`，每个工具一节。相同的文本只加一次。相对路径的 `@file` 导入会改成完整路径，这样从 vault 里也能用。工具自己往规则文件里追加的笔记也会被收进来
- **skill**：工具 skills 文件夹（`~/.claude/skills/`、`~/.agents/skills/`、`~/.qwen/skills/` 等）里新的 skill 文件夹会移到 `skills/`，原位置换成一个链接。完全相同的几份算同一个 skill。本身是链接、指向别的文件夹的 skill 留在原地；名字已经被另一个不同的 skill 占了的，也留在原地
- **工具自己的记忆**会复制（不是移动）到 `memory/`，这样每个工具都能搜到：
  - Claude Code 的自动记忆（`~/.claude/projects/*/memory/`），标上所属项目
  - Codex 的整合记忆（`~/.codex/memories/MEMORY.md` 和 `memory_summary.md`），每个任务组或每一节一条记忆。原始日志不导入
  - Qwen Code 的 `~/.qwen/memories/`
  - Hermes Agent 的 `~/.hermes/memories/MEMORY.md` 和 `USER.md`
  - DeepSeek Harness 的 `~/.dsh/lessons.md`，每个列表项一条记忆

  每份副本都有固定的 id，所以工具改写一条记忆时，vault 里的副本会原地更新，不会越堆越多重复。工具自己的记忆原样保留
- **看起来含有密码或密钥的记忆不会导入**，sync 会列出这些文件，你可以手动复制想要的内容。这个检查是模式匹配，可能漏掉真的密钥，也可能误报无害的内容

最后，工具文件里只剩 ai vault 区块。`aivault remove` 会把原来的规则文本和 skill 文件夹放回去。vault 保留自己的副本，包括同步过来的记忆。

## vault 里有什么

```
ai-vault/
  AGENTS.md     rules every tool loads
  me.md         your preferences
  *.md          any other file you put here loads too (say remember.md, mistakes.md)
  index/        lookup lists, searched line by line (where your files are, what your nicknames mean)
  memory/       one .md file per memory
  skills/       <name>/SKILL.md
```

vault 顶层的每个 `.md` 都属于每个工具都会加载的规则：先 AGENTS.md，再 me.md，其余按 a 到 z 排。这些文件要写短，因为每个工具每次会话都会把它们全读一遍。大一点的列表放进 `index/`：`vault_search` 只从里面返回匹配的那一行，连同它上面的标题，所以就算是一份 300 行的文件清单，问“我的化学第 4 单元练习卷”也只会给你一个路径。

它就是普通的 markdown，用 obsidian 或任何编辑器都行。想在多台机器上用，就把文件夹放进 icloud、dropbox、syncthing 或一个 git 仓库。

## 命令

| | |
| --- | --- |
| `aivault` | 连了哪些工具、vault 里有什么、最近存了什么 |
| `aivault init` | 设置 vault，连接工具 |
| `aivault doctor` | 全面检查（别名：`status`、`health`） |
| `aivault doctor --fix` | 重新连接出问题的工具，刷新过期的副本，然后再检查一遍 |
| `aivault sync` | 改完 vault 后刷新所有工具，并把它们那边的新东西拉进来（规则文本、skill、它们自己的记忆） |
| `aivault autosync on\|off\|status` | 从登录起就在后台保持所有东西同步 |
| `aivault watch` | 同样的事，放在前台跑：规则文件一改就刷新工具（加 `--sync 1h` 还会每小时完整同步一次） |
| `aivault search <query>` | 搜索记忆、skill 和 `index/` 列表 |
| `aivault add [--file remember] <text>` | 从终端存一条记忆，同时也写进一个 vault 文件 |
| `aivault open [file]` | 打开 vault，或者其中一个文件（`remember`、`memory`、`skills` 等） |
| `aivault import` | 把已有的规则和 skill 移进 vault |
| `aivault remove [tool...]` | 断开工具连接，不指定就全部断开 |
| `aivault update` | 更新 aivault 本身（`--check` 只看不装） |
| `aivault completion <shell>` | bash、zsh、fish 或 powershell 的 tab 补全 |
| `aivault mcp` | 工具启动的 mcp 服务器 |

### 首页

直接运行 `aivault`：

```
$ aivault
aivault  0.2.0

  vault    ~/ai-vault
  tools    ✓ Claude Code  ✓ Codex  ✓ OpenCode
           not connected: Gemini CLI. aivault init

  memory   229, 12 this week
           claude 140 · codex 61 · opencode 28
  skills   219
  index    3 lists, 105 entries
  rules    6 files, 22 KB, loaded by every tool each session
  synced   2 h ago, autosync on

  saved    ▁▁▃▁▂▁▁▂▁▁▁▁▁▅▁▁▂▇▂▁█▁▆▁▁▁▁▅▁▂▃▁▂▁▁▁▃▁▁▂▁▁▅▃▁▂▂▆▄▁█▁▁▄▁▁
           sep 3                                              today
  latest   2 h    claude  the deploy script needs node 24, not 26
           5 h    codex   school files are in ~/Documents/school
           1 d    claude  short commit titles, no trailers

  aivault help   aivault search <words>   aivault doctor
```

柱状图是最近 8 周每天存下的记忆数。它只读本地文件，所以一下就出来。

### doctor

```
$ aivault doctor
aivault
  · version           0.2.0
  ✓ binary            /Users/me/.local/bin/aivault
  ✓ update            latest

vault
  ✓ folder            /Users/me/ai-vault
  ✓ rules             6 files, 22 KB: AGENTS.md, me.md, dont.md, mistakes.md, portfolio.md, remember.md
  · contents          229 memories, 219 skills, 3 index lists
  ✓ secrets           nothing that looks like a key or password

mcp
  ✓ server            starts, lists both tools, search works (28 ms)

tools
  ✓ Claude Code       rules and mcp in place
  ✗ Codex             ~/.codex/AGENTS.md has an old copy of the vault rules. run aivault sync
  · Gemini CLI        installed but not connected. aivault init connects it
  ✓ skill links       543 links

sync
  · last sync         2 h ago
  · autosync          off. aivault autosync on keeps every tool current by itself

8 ok · 1 problem

aivault doctor --fix reconnects tools and refreshes copies
```

mcp 检查是真跑的：doctor 用和你的工具一样的方式启动服务器，列出它的工具，再跑一次搜索。`--json` 输出同样的报告给脚本用，`--quick` 跳过 mcp 测试和更新检查。有东西坏了，它会以非零状态码退出。

### autosync

`aivault autosync on` 会装一个小的后台任务：mac 上是 launchd agent，linux 上是 systemd 用户服务，windows 上是登录时运行的任务。它跑的是 `aivault watch --sync 1h`：改了规则文件，几秒内每个工具都能收到；每小时还会完整同步一次，把工具存进自己记忆里的东西拉进来。日志在 `~/.aivault/watch.log`（linux 上用 `journalctl --user -u aivault`）。`aivault autosync off` 会把它删掉。

### shell 补全

```sh
aivault completion zsh > "${fpath[1]}/_aivault"       # zsh
aivault completion bash > ~/.local/share/bash-completion/completions/aivault
aivault completion fish > ~/.config/fish/completions/aivault.fish
aivault completion powershell >> $PROFILE              # powershell
```

## 支持的工具

2026-10-19 对照每个工具的文档或源码核对过。`~` 是你的主目录（windows 上是 `%USERPROFILE%`）。

| 工具 | 规则写到哪 | mcp 服务器写到哪 |
| --- | --- | --- |
| Claude Code | `~/.claude/CLAUDE.md`（导入 vault 文件） | `claude mcp add --scope user` |
| Codex | `~/.codex/AGENTS.md`（副本） | `~/.codex/config.toml` |
| OpenCode | `~/.config/opencode/opencode.json(c)` 里的 `instructions` | 同一个文件，`mcp` |
| Gemini CLI | `~/.gemini/GEMINI.md`（导入） | `~/.gemini/settings.json` |
| Kilo Code | `~/.config/kilo/kilo.json(c)` 里的 `instructions` | 同一个文件，`mcp` |
| Crush | `~/.config/crush/crush.json` 里的 `options.global_context_paths` | 同一个文件，`mcp` |
| Goose | `~/.config/goose/.goosehints`（副本） | `~/.config/goose/config.yaml` 里的 `extensions` |
| Pi | `~/.pi/agent/AGENTS.md`（副本） | `~/.pi/agent/mcp.json` |
| Hermes Agent | `~/.hermes/SOUL.md`（副本） | `~/.hermes/config.yaml` 里的 `mcp_servers` |
| Cline | `~/.cline/rules/ai-vault.md`（副本） | `~/.cline/data/settings/cline_mcp_settings.json` |
| Zed | `~/.config/zed/AGENTS.md`（副本） | `~/.config/zed/settings.json` 里的 `context_servers` |
| Warp | `~/.agents/AGENTS.md`（副本） | `~/.warp/.mcp.json` |
| Open Interpreter | `~/.openinterpreter/AGENTS.md`（副本） | `~/.openinterpreter/config.toml` |
| Aider | `~/.aider.conf.yml` 里的 `read:` | （aider 没有 mcp） |
| Qwen Code | `~/.qwen/QWEN.md`（导入） | `~/.qwen/settings.json` |
| Kimi Code | `~/.kimi-code/AGENTS.md`（副本） | `~/.kimi-code/mcp.json` |
| DeepSeek Harness | `~/.dsh/AGENTS.md`（副本） | `~/.dsh/cordis.patch.yml` 里的一个 `insert` patch |
| ZCode | `~/.zcode/AGENTS.md`（副本） | `~/.zcode/cli/config.json` 里的 `mcp.servers` |
| MiniMax Code | `~/.minimax/AGENTS.md`（副本） | `~/.minimax/mcp.json` |
| Kode | （kode 没有全局规则文件） | `~/.kode.json` |
| Neovate Code | `~/.neovate/AGENTS.md`（副本） | `~/.neovate/config.json` |

skill 会链接到 `~/.agents/skills/`，上面大部分工具都读这个目录。claude code、hermes、kode 和 neovate 自己的 skills 文件夹也会链接上。

“导入”是说工具自己加载 vault 文件，所以改动在下次会话就会生效。“副本”是说工具没法导入文件，所以它拿到的是一份规则副本：改完 vault 要跑 `aivault sync`。

windows 上，goose、zed 和 hermes 把配置放在 `%APPDATA%` 或 `%LOCALAPPDATA%` 下。aivault 会照着来，也认那些用来挪动这些文件夹的环境变量（`CLAUDE_CONFIG_DIR`、`CODEX_HOME`、`XDG_CONFIG_HOME`、`GEMINI_CLI_HOME`、`QWEN_HOME`、`KIMI_CODE_HOME`、`DSH_HOME`、`HERMES_HOME`、`PI_CODING_AGENT_DIR`、`CLINE_DIR`、`KILO_CONFIG_DIR`、`CRUSH_GLOBAL_CONFIG`、`GOOSE_PATH_ROOT`、`INTERPRETER_HOME`、`MINIMAX_DATA_DIR`、`KODE_CONFIG_DIR`）。

没有支持的，以及原因：
- iFlow CLI 已关停（2026-04-17）。Roo Code 已关停（2026-05-15）。Void 已归档
- Continue、OpenHands CLI 和旧的 Python 版 Kimi CLI 已经没人维护了
- Trae Agent 只读当前文件夹里的配置，没有全局的东西可以连
- Copilot CLI、Cursor CLI、Amp、CodeBuddy 和 Comate 不开源。Claude Code 也不开源，但用的人实在太多，所以还是放进来了

## 注意事项

aivault 负责把 vault 接进去，但没法让模型一定去用它。依赖它之前先读完这部分。

**这里没有任何保证**
- 每个连上的工具都会拿到 vault 规则和两个 vault 工具，但什么时候调用 `vault_search` 和 `vault_remember` 由模型决定。默认规则催得很紧（回答任何关于你的问题前先搜索，你一说“记住”就保存），大多数模型都会照做，但总有些回答会跳过 vault
- 拿到规则副本的工具（codex、pi、hermes、kimi、dsh，以及上表里标了“副本”的其他工具）要等 `aivault sync` 之后才看得到你自己的改动，除非开了 autosync。用 `vault_remember` 加 `file` 添加的行会马上更新它们的副本
- vault 顶层的每个文件都会进每个工具的上下文。文件一大，每次会话都要多花 token，还可能让工具超出大小上限。Hermes 和 DeepSeek Harness 有上限（见下文），规则快被截断时 `doctor` 会提醒
- 工具在会话开始时加载规则。`init` 或 `sync` 之后要重启它们
- 配置之后才装的工具，要再跑一次 `aivault init` 才会连上
- 工具存进自己记忆的东西，要到下次同步才会进 vault，不是马上。开了 autosync 的话，一小时内就会到。没开的话，没有任何东西会替你跑 sync
- 搜索就是普通的关键词匹配，不是语义搜索。记忆的措辞和问题不一样时，可能搜不到
- `vault_remember` 会拒绝看起来像密码或密钥的文本，但这个检查只能抓到明显的格式。vault 就是普通的 markdown，你放在哪它就同步到哪（icloud、dropbox、git），所以别在里面存密钥或密码

**如果你已经有一套配置**
- 先看看有没有别的安装器或 dotfiles 脚本在管同样的文件（`~/.claude/CLAUDE.md`、`~/.codex/AGENTS.md` 等）。aivault 会加上它自己带标记的区块，而守着这些文件的脚本下次运行时可能会拒绝执行，或者把这个区块覆盖掉。先停用那个脚本，或者把这些文件排除掉
- 先备份。`aivault remove` 能撤销 aivault 自己的改动，但它不知道别的工具在之前或之后做了什么
- `init --import` 会移动规则文本和 skill 文件夹。如果 skill 是指向别处的链接，或者已经有另一个同名的 skill，它就不动这些 skill。看清它输出了什么

**各个工具**
- **OpenCode**：如果你的 `opencode.json` 里有 skill 白名单（`"permission": {"skill": {"*": "deny", ...}}`），不在名单上的 vault skill 会一直被拦着。aivault 不会改你的权限规则：`init` 和 `doctor` 会告诉你有多少个 vault skill 被拦了，列出其中几个，并给出要加的那一行。OpenCode 也会读 `~/.claude/skills` 和 `~/.agents/skills`，所以同一个 skill 可能列出两次
- **Codex**：它会拦下没人批准过的 mcp 调用，而 headless 运行时没法询问，所以 aivault 预先批准了自己的两个工具（`default_tools_approval_mode = "approve"`）。Codex 会完整读取 `~/.codex/AGENTS.md`，`project_doc_max_bytes` 只限制项目里的 AGENTS.md
- **Gemini CLI**：在你没信任过的文件夹里，所有 mcp 服务器都是关的，用户级的也一样。vault 规则照样会加载，但要信任这个文件夹之后 `vault_search` 才会出现。如果你让它走代理，Gemini CLI 还会调用它自己内置的模型名，代理得认得这些名字
- **Qwen Code**：“记住这个”往往会存进 Qwen 自己的记忆（`~/.qwen/memories/`），而不是 vault，就算规则里说了别这样也一样。`aivault sync` 会把这些记忆复制进 vault，所以它们要到下次同步之后才会到你的其他工具，不是马上
- **Hermes Agent**：`SOUL.md` 是它唯一的全局文件，所以 vault 规则就放在那里，挨着它的人设。`SOUL.md` 超过 `context_file_max_chars` 时，Hermes 会删掉中间部分，小模型上这个值是 20,000 个字符。规则快到这个长度时，`sync` 会在 `~/.hermes/config.yaml` 顶部设置 `context_file_max_chars`，被删掉了也会重新设上。你自己设的值会保留，太小的话 `doctor` 会告诉你
- **ZCode** 只在 `features.mcp` 打开时才加载 mcp 服务器。只有你从没设置过它，aivault 才会把它打开。你自己设的 `false` 会保留，`doctor` 会告诉你
- **Neovate** 自己也会读 `~/.claude/CLAUDE.md`，所以连了 Claude Code 的话，它会看到两遍规则
- **DeepSeek Harness**：`~/.dsh/cordis.patch.yml` 必须保持是一个 yaml 列表，不然 aivault 不会去改它。dsh 把 `~/.dsh/AGENTS.md` 和所有项目里的 AGENTS.md 一起塞进 64 KB，所以 vault 规则越大，留给项目文件的空间越少
- **Crush** 的许可证（FSL）是 source-available，不是开源
- **Gemini CLI** 从 2026-06-18 起不再支持免费账号。用 api key 还是能用
- 带空格的 vault 路径（比如 icloud 的 `Mobile Documents`）对 Claude Code 的导入没问题。Gemini 和 Qwen 这时拿到的是副本而不是导入，所以改完也要跑 `sync`
- windows 上不允许建链接时，skill 链接会退回成副本。改了 skill 之后要跑 `sync`
- windows 上，autosync 会在登录时通过一个隐藏的 powershell 窗口启动，可能会闪一下

**真实测过的**（2026-10-20，提示词里完全没提 vault）：Claude Code、Codex、OpenCode、Pi 和 DeepSeek Harness 都根据 vault 规则作答，找到了只存在于某条 vault 记忆里的事实，还存了一条新记忆。Qwen Code 过了前两项，它存进自己记忆的东西在一次同步后出现在了 vault 里。Claude Code 还找到了一条只来自 Hermes 自身记忆的记忆。其他工具靠单元测试检查，装了的再用它们自己的 `mcp list` 检查，没有用真实模型跑过

## 它会改什么

只改它标记为自己的文本（json 文件里则是它自己的键），而且每个文件都会先备份（`~/.aivault/backups/`）。

`aivault remove` 会全部撤销。如果之后你没动过某个文件，就能逐字节拿回原文件。如果动过，只会删掉 ai vault 的部分。vault 文件夹本身永远不会被删。

## agent 能拿到两个工具

- `vault_search`：按关键词查找记忆、skill 和 `index/` 里的行
- `vault_remember`：保存一条记忆，并标上是哪个工具存的。带上 `file`（比如 `remember` 或 `mistakes`）时，还会把这一行加进对应的 vault 文件，顶层的或 `index/` 里的都行。这个文件必须已经存在，这样打错字也不会凭空建出一个所有工具都会加载的新文件。没法传 `file` 的工具，可以改成在文本开头写文件名（`remember: ...`）。看起来像密码或密钥的文本，它会拒绝

mcp 故意没有删除功能。记忆文件要删就自己删。

## 开发

```sh
go test ./...
go run ./cmd/aivault
```

发布：推一个像 `v0.1.0` 这样的 tag。release workflow 会运行 goreleaser，它会构建所有平台、创建 github release，并更新 brew cask 和 scoop manifest。它需要一个能写入 `jxherc/homebrew-tap` 和 `jxherc/scoop-bucket` 的 `TAP_TOKEN` secret。本地可以用 `goreleaser release --snapshot --clean` 试一下。

v1 的计划见 [SPEC.md](SPEC.md)。

## 许可证

MIT，见 [LICENSE](LICENSE)。
