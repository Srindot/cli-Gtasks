# ✿ gtask

Google Tasks in your terminal: quick, Rosé Pine colors, transparent background, nvim keys.

```
gtask --demo     # try it with sample data, no account needed
gtask auth       # connect Google (one-time setup below)
gtask            # open the TUI
gtask ls [list]  # print open tasks
gtask add -l work "ship it @fri"
```

## How it works

- **Instant:** it opens from a local cache (`~/.cache/gtask`). Every change shows up right away and syncs in the background, in order. Quitting waits for pending changes to finish syncing.
- **Transparent:** only foreground colors are used, so your terminal background shows through.
- **Animated:** completing a task fills the checkbox and sweeps a strikethrough across it. Deleted rows fade out in red. The cursor leaves a short trail. List titles type themselves in. The logo shimmers while syncing. Messages slide in and fade out. When nothing is moving, the app uses no CPU.

## Keys (press `?` in the app)

| | |
|---|---|
| `j k` `gg G` `^d ^u` `5j` | move |
| `h l` `tab` | switch between the lists and tasks panes |
| `H L` / `gT gt` | previous / next list |
| `o O` | new task below / above (Enter adds another, Esc finishes) |
| `i a r` · `cc S` | rename · replace the title |
| `e` / `⏎` | edit the title and notes in `$EDITOR` |
| `x` / `space` | mark done / not done |
| `t` · `T` | set / clear the due date (`tom`, `fri`, `+3d`, `oct 12`, …) |
| `dd` `yy` `p P` | delete / yank / paste (works across lists, so `dd` then `p` moves a task) |
| `J K` · `>> <<` | move down / up · indent / outdent |
| `za zR zM` | fold |
| `u` | undo |
| `/` · `C` · `R` | filter · show completed · sync now |
| `:new` `:rename` `:dellist` `:clear` `:q` | list commands |

Quick add: end a title with `@tomorrow`, `@fri`, or `@+2w` to set a due date.

## One-time Google setup

1. Create a project at https://console.cloud.google.com/projectcreate
2. Enable the **Tasks API**: https://console.cloud.google.com/apis/library/tasks.googleapis.com
3. In **Google Auth Platform**, open **Branding** and enter an app name and your email. Then open **Audience**, choose External, and add yourself as a test user.
   *If you leave the app in "Testing", tokens expire every 7 days. Click **Publish app** to avoid that. Google's "unverified app" warning is fine for personal use.*
4. Go to **Clients**, create a client of type **Desktop app**, and download the JSON.
5. Save it as `~/.config/gtask/credentials.json`, then run `gtask auth`.
