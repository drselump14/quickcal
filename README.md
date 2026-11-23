# quickcal

Natural-language quick event entry for [khal](https://github.com/pimutils/khal), as a terminal UI.

Type an event the way you'd say it, see a live preview with conflict checks against all your
calendars, press enter, and it's added with `khal new`.

```
Lunch with Ana at Sushiro tomorrow 12:30 for 90m /work
Dentist next fri 3pm remind 30m
Vacation dec 24 - dec 28 /icloud
Gym every monday 7pm until 2027-03-01
Call mom in 2 hours
Review PR 15:00-16:30 :: check migrations
明日15時 歯医者                     (via aichat fallback)
```

## Features

- **Hand-written parser** (no dependencies): dates, times, ranges (`3-4pm`, `from 11 to 1`),
  durations (`for 90m`), location (`at X`), calendar (`/name`, prefix match), repeat
  (`every mon … until DATE`), alarm (`remind 15m`), description (`:: notes`).
- **Conflict detection** across all calendars as you type; all-day events (holidays) never conflict.
- **AI fallback** via [aichat](https://github.com/sigoden/aichat), only when the parser likely
  missed something (non-English text, Indonesian date words, leftover numbers). The `/calendar`
  token and `:: notes` are never sent.
- **Vi mode** for the input (motions, `d`/`c`/`y` operators, counts, `u` / `ctrl+r` undo/redo).
- **kitty integration**: `ctrl+o` opens `ikhal` in a split; `--fit` sizes (and, with yabai,
  centers) its own window.

## Install

```sh
go install github.com/drselump14/quickcal@latest
```

Needs `khal` on PATH. Optional: `ikhal`, `aichat`, kitty with `allow_remote_control yes`, yabai.

## Usage

```sh
quickcal                 # TUI
quickcal --fit           # TUI, resize own kitty window to fit
quickcal -n "gym mon 7pm /work"   # print the khal command (dry run)
quickcal -x "gym mon 7pm /work"   # create the event directly
```

| Key | Action |
|---|---|
| `enter` | add event |
| `esc` / `i a I A` | normal / insert mode |
| `u` `ctrl+r` `ctrl+z` | undo / redo (undo also restores input cleared after adding) |
| `ctrl+t` | cycle calendar |
| `ctrl+a` | force AI parse |
| `ctrl+o` | open / close ikhal |
| `ctrl+l` | refresh events |
| `ctrl+q` / `ctrl+c` | quit |

| Env | Effect |
|---|---|
| `QUICKCAL_AI=off` | disable AI fallback |
| `QUICKCAL_AI_MODEL` | aichat model (default: aichat's default) |
| `QUICKCAL_IKHAL_AT` | `vsplit` (default), `hsplit`, `tab`, `os-window`, `overlay` |

Parser conventions: `12/5` is month/day, a bare `at 3` means 15:00, a time already past today
moves to tomorrow, `next fri` is the next Friday after today.
