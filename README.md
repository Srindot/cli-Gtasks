# ✿ gtask

Google Tasks in your terminal: quick, Rosé Pine colors, transparent background, nvim keys.

```
gtask --demo     # try it with sample data, no account needed
gtask setup      # connect Google: guided, one time
gtask            # open the TUI
gtask ls [list]  # print open tasks
gtask add -l work "ship it @fri"
```

## How it works

- **Instant:** it opens from a local cache (`~/.cache/gtask`). Every change shows up right away and syncs in the background, in order. Quitting waits for pending changes to finish syncing.
- **Transparent:** only foreground colors are used, so your terminal background shows through.
- **Animated:** completing a task fills the checkbox and sweeps a strikethrough across it. Deleted rows fade out in red. The cursor leaves a short trail. List titles type themselves in. The logo shimmers while syncing. Messages slide in and fade out. When nothing is moving, the app uses no CPU.

## Layout

The layout follows yazi, with three columns: **lists** on the left, **tasks and subtasks** in the middle, **details** on the right (deadline, status, notes, subtasks). Use `h`/`l` to move between them.

## Keys (press `?` in the app)

| | |
|---|---|
| `j k` `gg G` `^d ^u` `5j` | move |
| `h l` · `tab` | lists ← tasks → details · cycle panes |
| `H L` / `gT gt` | previous / next list |
| **`a`** | new task (input box; `tab` switches to subtask) |
| **`A`** | new subtask of the selected task |
| `o O` | type a new task inline below / above (Enter keeps adding) |
| `x` / `space` | mark done / not done |
| `r` · `i` · `S` | rename (input box) · rename inline · replace the title |
| `t` · `T` | set the deadline (with a live preview) · clear it |
| `N` · `e` | edit notes inline · edit the title and notes in `$EDITOR` |
| `l` then `j/k` `⏎` | edit any field in the details pane (title, deadline, status, notes, subtasks) |
| `dd` `yy` `p P` | delete / yank / paste (works across lists) |
| `J K` · `>> <<` | move down / up · make / unmake a subtask |
| `za zR zM` | fold |
| `u` | undo |
| `/` · `C` · `R` | filter · show completed · sync now |
| lists pane: `a` `r` `dd` | new / rename / delete a list |

Icons come from a Nerd Font. Set `GTASK_ICONS=0` for plain unicode instead.

Quick add: end a title with `@tomorrow`, `@fri`, or `@+2w` to set a due date.

## One-time Google setup

Run `gtask setup`. It walks through five short steps and opens the exact console page for each one, so you never have to find your way around the Cloud console:

1. **Create a project.** Any name works.
2. **Enable the Tasks API.**
3. **Set up the sign-in screen.** Click Get started, choose External, and enter your email.
4. **Publish the app.** If you skip this, Google signs you out every 7 days.
5. **Create a Desktop app client.** Then paste its Client ID and secret into the terminal.

Then it signs you in. When Google says *"Google hasn't verified this app"*, click **Advanced → Go to gtask (unsafe)**. That's expected, because the app is your own.

If you already downloaded a `client_secret_*.json`, `setup` finds it in `~/Downloads` and skips straight to sign-in.
