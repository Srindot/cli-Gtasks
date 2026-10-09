<div align="center">

# ✿ gtask

**Google Tasks in your terminal.**
Three panes like [yazi](https://github.com/sxyazi/yazi), keys like Neovim, colors from [Rosé Pine](https://rosepinetheme.com).

[![CI](https://github.com/Srindot/cli-Gtasks/actions/workflows/ci.yml/badge.svg)](https://github.com/Srindot/cli-Gtasks/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/go-1.26-9ccfd8?logo=go&logoColor=white)

</div>

```
 ✿ gtask  Inbox / Plan weekend trip / Book train tickets                       demo · ● synced just now
 ▾ Inbox            7 │  ○ Water the plants                  ◷ today │
 ▸ Work             3 │  ○ Plan weekend trip ≡                 ◷ Tue │  ↳ Plan weekend trip
 ▸ Reading          2 │▎  ├─ ○ Book train tickets                    │  Book train tickets
                      │   ╰─ ○ Pack the film camera                  │
                      │  ○ Call mom                       ◷ tomorrow │  ◷ due     none
                      │  ○ Renew passport               ◷ 2d overdue │  ● status  ○ open
                      │  ○ Try the new ramen place                   │
                      │                                              │  ≡ notes ──────────────────────
                      │                                              │  empty — ⏎ to write
 NOR  Inbox  7 open · 2 done                                                          ? help   3/7  All
```

## Features

- **Fast.** It opens straight from a local cache. Every change shows up immediately and syncs to Google in the background, in order. Quitting waits until pending changes have been saved.
- **Three panes.** Your lists sit on the left, tasks and subtasks in the middle, and the right pane shows the selected task's deadline, status, notes and subtasks.
- **Vim keys.** You get `hjkl`, counts like `5j`, `dd`/`yy`/`p`, `>>`/`<<`, folds, undo, `/` filter and `:` commands.
- **Easy to create tasks.** Press `a` for a new task or `A` for a subtask. An input card opens in the list, right where the task will go, and pushes the rows below it down. Type `@fri` or `@+2w` at the end of the title for a due date: it turns gold, and the date appears in the card's bottom edge.
- **Transparent.** It only sets text colors, so your terminal's background, blur and opacity show through.
- **Animated, not distracting.** Ticking off a task fills its checkbox and strikes through the title, and deleted rows fade out in red. Input cards draw their own border in, the caret softly pulses, and each character you type glows as it appears. New tasks briefly glow gold, and the panes slide and fade in. When nothing is moving, gtask uses no CPU.
- **Scriptable.** `gtask ls` and `gtask add` work without opening the full-screen app.

## Install

You need [Go](https://go.dev/dl/) 1.26 or newer.

```sh
go install github.com/Srindot/cli-Gtasks/cmd/gtask@latest
```

Or build it from source:

```sh
git clone https://github.com/Srindot/cli-Gtasks && cd cli-Gtasks
go build -o gtask ./cmd/gtask
```

The icons need a [Nerd Font](https://www.nerdfonts.com). Without one, run `GTASK_ICONS=0 gtask` to use plain Unicode symbols.

## Quick start

```sh
gtask --demo       # look around with sample data; no account needed
gtask setup        # connect your Google account (guided, one time)
gtask              # open your tasks
```

From the command line:

```sh
gtask ls [list]                      # print open tasks
gtask lists                          # print your task lists
gtask add -l work "ship it @fri"     # add a task with a due date
gtask add -n "notes here" "call mom @tomorrow"
gtask logout                         # forget the saved sign-in
```

## Connecting Google

Google requires every app that reads your tasks to have its own key, so you create one for yourself. Run `gtask setup`. It opens the exact Cloud console page for each step and lists what to click:

| Step | Page | What to do |
|---|---|---|
| 1 | [Create a project](https://console.cloud.google.com/projectcreate) | Name it `gtask` → **Create**. Wait for the bell icon to say it's ready. |
| 2 | [Tasks API](https://console.cloud.google.com/apis/library/tasks.googleapis.com) | Check that the project picker says *gtask* → **Enable**. |
| 3 | [Sign-in screen](https://console.cloud.google.com/auth/overview) | **Get started**. App name `gtask`, your email → **External** → your email → agree → **Create**. |
| 4 | [Audience](https://console.cloud.google.com/auth/audience) | Leave the app in *Testing*. Under **Test users**, click **+ Add users**, add your Gmail address and **Save**. |
| 5 | [New client](https://console.cloud.google.com/auth/clients/create) | Application type: **Desktop app** → **Create** → **Download JSON**. |

gtask picks up the JSON from `~/Downloads` by itself, then opens your browser so you can sign in. When Google says *"Google hasn't verified this app"*, click **Advanced → Go to gtask (unsafe) → Continue**. The warning is expected, because the app is your own.

**About the weekly sign-in.** Google makes apps in *Testing* sign in again about every 7 days. When that happens, gtask notices on startup and opens the browser for you, so it takes a couple of clicks. You can avoid it by clicking **Publish app** on the Audience page. If Google says *"complete your configuration on the Branding page"*, open [Branding](https://console.cloud.google.com/auth/branding), fill in the app name, user support email and developer contact email, **Save**, and then publish.

**Cost.** The Tasks API is free, and the project never needs a billing account. Google's default [quota](https://developers.google.com/workspace/tasks/limits) is 50,000 requests a day. gtask syncs every 90 seconds, and only while it's open, so it stays far below that.

## Keys

Press `?` inside the app for the full list.

| Keys | Action |
|---|---|
| `j` `k` · `gg` `G` · `^d` `^u` · `5j` | move |
| `h` `l` · `tab` | lists ← tasks → details · cycle panes |
| `H` `L` · `gT` `gt` | previous / next list |
| **`a`** | new task: a card opens right where it will land (`tab` switches to a subtask) |
| **`A`** | new subtask of the selected task |
| `o` `O` | type new tasks inline below / above (Enter keeps adding) |
| `x` · `space` | mark done / not done |
| `r` · `i` · `S` | rename · edit the title inline · replace the title |
| `t` · `T` | set the due date (with a live preview) · clear it |
| `N` · `e` | edit notes inline · edit the title and notes in `$EDITOR` |
| `l` then `j` `k` `⏎` | edit any field in the details pane |
| `dd` · `yy` · `p` `P` | delete · yank · paste (works across lists) |
| `J` `K` · `>>` `<<` | move down / up · make / unmake a subtask |
| `za` `zR` `zM` | fold |
| `u` | undo |
| `/` · `C` · `R` | filter · show completed · sync now |
| `:q` `:new` `:rename` `:clear` | commands |
| in the lists pane: `a` `r` `dd` | new / rename / delete a list |

**Due dates.** You can type `today`, `tom`, `fri`, `next week`, `+3d`, `+2w`, `oct 12` or `2026-12-01`.

## Privacy

gtask talks only to Google's Tasks API. It has no telemetry and runs no server of its own. Everything it stores stays on your machine, and the files are readable only by you:

| File | Contents |
|---|---|
| `~/.config/gtask/credentials.json` | your OAuth client (from `gtask setup`) |
| `~/.config/gtask/token.json` | your sign-in token (`gtask logout` deletes it) |
| `~/.cache/gtask/cache.json` | an offline copy of your tasks |
| `~/.cache/gtask/crash.log` | stack traces, written only if something goes wrong |

These paths follow `$XDG_CONFIG_HOME` and `$XDG_CACHE_HOME` if you've set them. You can also skip `credentials.json` and set `GTASK_CLIENT_ID` and `GTASK_CLIENT_SECRET` in your environment.

## Development

```sh
go test ./...                # includes a randomized stress test of the UI
go run ./cmd/gtask --demo
```

The code is split into three parts:
- `internal/gtasks` holds the REST client, the OAuth flow and the in-memory demo backend.
- `internal/ui` is the [Bubble Tea](https://github.com/charmbracelet/bubbletea) app.
- `cmd/gtask` is the command-line entry point and the setup wizard.
