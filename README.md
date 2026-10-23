# quadrant

A minimal **macOS Apple Silicon (ARM64)** grid window placer, written in **Go 1.26**. Press a global shortcut to move and resize the **currently focused window** to a configured region of a configurable rows × columns grid on its current display.

- Pure Go (`CGO_ENABLED=0`): no Swift, Objective-C source, or cgo.
- Uses Carbon hotkeys, macOS Accessibility APIs and `NSScreen.visibleFrame` via Go bindings.
- Uses the usable display area, respecting the menu bar and Dock.
- Supports multiple monitors. The target is the monitor with the largest overlap with the focused window.
- Runs as an accessory application (`LSUIElement`, no Dock icon): the only UI is a menu-bar icon, whose menu holds **About Quadrant**, **Accessibility Settings…** and **Quit Quadrant** (⌘Q). No automatic tiling.
- Ships as a **`Quadrant.app` bundle**, which is what lets macOS grant it Accessibility under its own name; an executable outside a bundle can only inherit the grant of whatever launched it and never appears in that list.
- Starts without the grant rather than exiting: the menu-bar icon needs no permission, so the menu stays reachable, and the shortcuts are claimed the moment the grant is given — no restart. See [Accessibility permission](#accessibility-permission).
- First run creates `~/.quadrant/configuration.json`; existing configuration is never overwritten. The configuration format is intentionally breaking: old named-placement files and the escaped `++` / `\+` literal-plus spellings are rejected and must be edited manually.
- Re-reads that file every 30 seconds, so grid, shortcut, region, and padding changes apply without a restart. An edit that does not parse or validate is ignored and the running configuration is kept; see [Reloading](#reloading).
- The default layout is a 2×2 grid with the four quadrants and four halves expressed as cells and spans. Any `rows` × `columns` grid and rectangular region can be configured; see [Configuration](#configuration).

## Build

Requires macOS on Apple Silicon and Go **1.26 or newer**. An Internet connection is needed for the initial module download.

```sh
unzip quadrant.zip
cd quadrant
./build.sh
./install.sh
```

`install.sh` copies the finished bundle to `/Applications/Quadrant.app` and handles the Accessibility reset; `open dist/.build/Quadrant.app` runs the build without installing it.

`build.sh` detects a Rosetta-translated shell and re-executes itself under arm64, so a plain `./build.sh` works without `arch` or `GOFLAGS` overrides. It resolves dependencies, runs tests, then creates **`./dist/.build/Quadrant.app`**: an `.app` bundle holding the executable in `Contents/MacOS/Quadrant` and [`packaging/Info.plist`](packaging/Info.plist) in `Contents/`. The `.build` directory is hidden on purpose: Spotlight does not index hidden items, and Spotlight's application index is where the macOS launcher gets its icons — built in the open, the build output is a second “Quadrant” in that list beside the installed one. (Opening it from the Finder registers it with LaunchServices anyway, so run the installed copy rather than that one.) This ZIP ships source, tests and the build script; it does **not** contain a prebuilt application or vendored modules. Future builds use the Go module cache.

The bundle is signed during the build. [Signing](#signing) explains what that signature is for and how to make the Accessibility grant survive rebuilds.

The application icon is generated during the build, from the program's own drawing of it: `cmd/icongen` renders the mark at every size an icon set needs, and `iconutil` turns that into `Contents/Resources/AppIcon.icns` — before the bundle is signed, so the icon is inside the seal. Both steps need macOS, and a bundle built without them has no icon rather than a broken one.

The About panel shows the version `git describe --tags --always --dirty` reports, or `dev` when there is no repository to ask. The bundle's `CFBundleShortVersionString` is the numeric part of the same description (`1.2.0`), or `0.0.0` when there is none.

## Signing

The bundle is **ad-hoc signed** by default. That is enough to run it here, and it is what the Accessibility grant is checked against — but an ad-hoc signature is a hash of the code, so **every rebuild is a different application to macOS**, the stored grant stops matching, and the permission has to be given again. That is the difference between these two lines, which are what macOS records and re-checks:

```text
ad-hoc        designated => cdhash H"e4d30f5d…"                                    changes every build
self-signed   designated => identifier "com.quadrant.app" and certificate leaf = H"7913…"   does not
```

A **self-signed certificate is free** and gives you the second line. No Apple account, nothing uploaded, no trust setting changed:

```sh
./packaging/make-signing-identity.sh                       # once; your login keychain
QUADRANT_SIGN_IDENTITY="Quadrant Local Signing" ./build.sh
```

The script makes a private key and a code-signing certificate and puts them in the login keychain. `security find-identity` reports it as `CSSMERR_TP_NOT_TRUSTED`, which is expected — it is not Apple's certificate, and signing does not need it to be trusted. The first build that uses it may ask for permission to use the key: **Always Allow** that once. `QUADRANT_SIGN_KEYCHAIN=/path/to.keychain` names a different keychain if the identity lives elsewhere.

Grant Accessibility once after signing with it, and later rebuilds keep that grant — the requirement names the certificate, which does not change, instead of the code, which does. To go back, or to start over:

```sh
security delete-identity -c "Quadrant Local Signing"
```

A **Developer ID** certificate produces the same shape of requirement, anchored to Apple, and is the right one if anyone else is to run the app: Gatekeeper rejects a downloaded copy signed ad-hoc *or* with a local certificate, and notarizing — which is what removes that obstacle — needs a Developer ID. For a build that only ever runs on your own Mac, the free identity is the whole story:

```sh
QUADRANT_SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" ./build.sh
```

## Install

Build, then install:

```sh
./build.sh
./install.sh
```

`install.sh` stops a running Quadrant, copies `dist/.build/Quadrant.app` to `/Applications/Quadrant.app`, verifies the copied signature, and relaunches the app. When the build is ad-hoc signed — the default — it also resets the Accessibility grant for `com.quadrant.app`, because the new signature no longer matches the old one, and opens the Accessibility pane. Switch Quadrant on there; the app polls every two seconds and registers the shortcuts without a restart.

Use a different destination with `--dest`:

```sh
./install.sh --dest "$HOME/Applications"
```

If you use the sample LaunchAgent, install with `--dest "$HOME/Applications"` or change the plist's program path to `/Applications`. `install.sh` only restarts a loaded agent whose program path matches the installed bundle; otherwise it opens the app directly and says so.

A stable signing identity (the local certificate or a Developer ID) gives the new bundle the same requirement, so the grant survives; `--keep-accessibility` skips the reset for those builds. `--no-relaunch` copies only.

The build output is still just an application, so the manual equivalent is copying it somewhere stable and — if you want it at login — loading the sample LaunchAgent:

```sh
./build.sh
mkdir -p ~/Applications
cp -R dist/.build/Quadrant.app ~/Applications/          # the path the sample LaunchAgent points at
codesign --verify --strict ~/Applications/Quadrant.app
```

Then, from [Launch at login](#launch-at-login):

```sh
sed -e "s|/Users/you|$HOME|g" launchd/com.quadrant.agent.plist \
  > ~/Library/LaunchAgents/com.quadrant.agent.plist
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.quadrant.agent.plist
```

`/Applications` works too and is what a Mac user expects to find; it is writable without `sudo` for an administrator account, but the sample plist spells out `~/Applications/Quadrant.app`, so a copy there needs that path replaced rather than the home directory substituted. Either way the bundle is what System Settings lists and grants Accessibility to, so it should live somewhere you will not move it: the grant follows the application, and the permissions pane remembers where it was.

`codesign --verify --strict` is worth running after any copy: a signature macOS rejects cannot hold an Accessibility grant, and that failure is much easier to read here than in System Settings.

To uninstall:

```sh
launchctl bootout gui/$(id -u)/com.quadrant.agent
rm -f ~/Library/LaunchAgents/com.quadrant.agent.plist
rm -rf ~/Applications/Quadrant.app
```

`~/.quadrant/configuration.json` and the log in `~/Library/Logs/quadrant/` are yours: nothing removes them. An earlier version of this project installed a loose executable at `~/bin/quadrant` instead; it is not used any more, and `rm ~/bin/quadrant` is the end of it.

Upgrading means building again and installing again:

```sh
./build.sh && ./install.sh
```

`install.sh` resets the stale Accessibility grant for an ad-hoc build and reopens the pane. Signing with a stable identity avoids the re-grant.

**Breaking change.** The configuration format uses top-level `rows` and `columns` with a `shortcuts` object keyed by key combination. The old named-placement object and the escaped `++` / `\+` literal-plus spellings are rejected with no automatic migration. The modifier identifiers are `control`, `command`, `shift`, and `option`; `ctrl`, `cmd`, `opt`, `alt`, `meta`, `ctl`, and glyph forms are invalid. Arrow keys are `left_arrow`, `right_arrow`, `up_arrow`, and `down_arrow`; `left`, `right`, `up`, `down`, `leftarrow`, and the other `*arrow` spellings are invalid. An existing file that uses any of those spellings fails validation and must be edited.

## Default shortcuts

| Shortcut | Placement |
| --- | --- |
| Control + Command + `;` | Top-left quadrant |
| Control + Command + `'` | Top-right quadrant |
| Control + Command + `.` | Bottom-left quadrant |
| Control + Command + `/` | Bottom-right quadrant |
| Control + Command + `←` | Left half |
| Control + Command + `→` | Right half |
| Control + Command + `↑` | Top half |
| Control + Command + `↓` | Bottom half |

The default grid is 2 rows × 2 columns. The four quadrants are single cells; the four halves are spans of both rows or both columns. In `configuration.json` the four arrow keys are written `left_arrow`, `right_arrow`, `up_arrow`, and `down_arrow`.

Each configured region is a rectangular group of grid cells. A quadrant is one cell of the default grid; a half spans both rows or both columns. macOS applications can reject moves or impose minimum window sizes; such cases are logged to stderr.

## Configuration

On the first invocation, the program automatically creates:

```text
~/.quadrant/configuration.json
```

Default content:

```json
{
  "rows": 2,
  "columns": 2,
  "shortcuts": {
    "control+command+;": {"row": 1, "column": 1, "row_span": 1, "column_span": 1},
    "control+command+'": {"row": 1, "column": 2, "row_span": 1, "column_span": 1},
    "control+command+.": {"row": 2, "column": 1, "row_span": 1, "column_span": 1},
    "control+command+/": {"row": 2, "column": 2, "row_span": 1, "column_span": 1},
    "control+command+left_arrow": {"row": 1, "column": 1, "row_span": 2, "column_span": 1},
    "control+command+right_arrow": {"row": 1, "column": 2, "row_span": 2, "column_span": 1},
    "control+command+up_arrow": {"row": 1, "column": 1, "row_span": 1, "column_span": 2},
    "control+command+down_arrow": {"row": 2, "column": 1, "row_span": 1, "column_span": 2}
  },
  "padding_points": 0
}
```

`rows` and `columns` are required positive integers. They define one uniform grid over the usable area of the focused window's display. `row` and `column` are required and one-indexed: row 1 is the top row and column 1 is the left column. `row_span` and `column_span` are optional and default to 1; the whole region must stay inside the grid. At least one shortcut is required. The order of entries in the `shortcuts` object is not significant, and a repeated JSON key is rejected rather than silently keeping the last target.

Bindings are written as one or more modifiers, a `+`, and exactly one key. The key token must be last; modifier order may be any order, so `control+command+1` and `command+control+1` are the same mapping. Shortcut keys are compared case-sensitively, with surrounding whitespace ignored and only modifier order normalized. The only modifier identifiers are `control`, `command`, `shift`, and `option`. There are no aliases: `ctrl`, `cmd`, `opt`, `alt`, `meta`, `ctl`, and the glyph forms are invalid. Multi-word identifiers use `_` between words. The arrow keys are `left_arrow`, `right_arrow`, `up_arrow`, and `down_arrow`; the short forms `left`, `right`, `up`, `down` and concatenated forms such as `leftarrow` are invalid. Other named keys are `space`, `return`, `tab`, `delete`, `escape`, and `f1`–`f15`.

A key may also be a letter, a digit, or a symbol. Letters are case-insensitive when resolving the physical key: `control+command+A` and `control+command+a` name the same key, and an uppercase letter does not imply Shift. Because shortcut-map comparison is case-sensitive, those two spellings cannot both appear; the physical-combination check rejects the pair. Write `shift` explicitly when you need it. A shifted symbol implies Shift, so `control+command+!` is `control+command+shift+1`. The literal `+` key is written `control+command+plus`; `control+command++` and `control+command+\+` are invalid. In JSON, a literal `\` or `"` may be written as `backslash` or `doublequote` to avoid escaping.

`padding_points` is the spacing between the window's outer edges and the boundaries of its assigned grid region: each of the four outer edges is inset by that many points (0–128). A spanning region receives a single outer inset, with no internal gutters. Two adjacent regions each inset their own window, so the visible gap between those windows is twice the padding value.

For example, this 4×4 grid defines two 2×2 regions — the top-left and top-right halves of the screen:

```json
{
  "rows": 4,
  "columns": 4,
  "shortcuts": {
    "control+command+1": {"row": 1, "column": 1, "row_span": 2, "column_span": 2},
    "control+command+2": {"row": 1, "column": 3, "row_span": 2, "column_span": 2}
  },
  "padding_points": 0
}
```

### Converting an old configuration

There is no automatic migration. For the old default layout, set `rows` and `columns` to 2, make each old binding the key of a `shortcuts` entry, and use this mapping for the region value:

| Old name | New region |
| --- | --- |
| `top_left` | `"row": 1, "column": 1, "row_span": 1, "column_span": 1` |
| `top_right` | `"row": 1, "column": 2, "row_span": 1, "column_span": 1` |
| `bottom_left` | `"row": 2, "column": 1, "row_span": 1, "column_span": 1` |
| `bottom_right` | `"row": 2, "column": 2, "row_span": 1, "column_span": 1` |
| `left_half` | `"row": 1, "column": 1, "row_span": 2, "column_span": 1` |
| `right_half` | `"row": 1, "column": 2, "row_span": 2, "column_span": 1` |
| `top_half` | `"row": 1, "column": 1, "row_span": 1, "column_span": 2` |
| `bottom_half` | `"row": 2, "column": 1, "row_span": 1, "column_span": 2` |

Bindings that used `control+command++` or `control+command+\+` must be rewritten as `control+command+plus`.

Validate configuration without running the hotkey service:

```sh
./dist/.build/Quadrant.app/Contents/MacOS/Quadrant -check-config
```

Use a different configuration file when developing:

```sh
./dist/.build/Quadrant.app/Contents/MacOS/Quadrant -config ./configuration.example.json -check-config
```

Running the executable itself rather than `open dist/.build/Quadrant.app` is how the command-line flags are reached: `-check-config` validates and exits before any permission is involved.

At least one shortcut is required, and all configured keys must be distinct after normalization. Modifier-order permutations are one mapping; two entries that resolve to the same physical hotkey are rejected. Every region must fit inside the configured grid. macOS reserves some plain Control+arrow combinations for Mission Control and Spaces; the Control+Command defaults above are not in the built-in symbolic-hotkey list this project checks, but a system or application shortcut can still overlap and is not always detectable. Startup and reload report a detectable conflict rather than silently changing the binding. Some foreground apps may have overlapping key equivalents which macOS cannot detect; customize bindings if necessary. Existing invalid configuration is never reset.

`-check-config` validates the file and its bindings; it does not register the hot keys, so a conflict with another application is reported when the program starts or reloads.

### Reloading

The configuration file is re-read every **30 seconds**. The poll itself is silent — only a successful reload is logged. What triggers a reload is the file's **content** changing, so saving an unchanged file does nothing, and neither does deleting it: a missing file keeps the configuration already in effect rather than resetting it to the defaults.

Content that has changed is loaded and validated before anything in the running program is touched:

- **Invalid file** — a file that does not parse, or that breaks one of the rules above, is never applied. The configuration in effect keeps running, the failure is reported once (a later poll of that same unchanged content stays quiet), and the file is tried again only after it changes.
- **Shortcut already taken** — a valid file whose bindings cannot all be registered is reported, and the previous shortcuts are restored, so you are never left with fewer shortcuts than you had.
- **Valid file** — the new shortcuts, grid, regions, and `padding_points` take effect on the next keypress, and one `configuration reloaded: <path>` line is logged.

`-check-config` validates and exits without watching. A file saved while the program is still starting up is applied by the first poll after it starts.

## Accessibility permission

Moving windows belonging to other apps requires macOS **Accessibility** permission, but global hotkey registration itself does not.

**The grant belongs to the application that asks for it, and only a bundle is an application.** An executable outside a `.app` is a command-line program to macOS: the grant goes to whatever launched it — the terminal — the program never appears in System Settings under its own name, and it cannot be granted the permission at all. That is why this project ships `Quadrant.app`, and why running a copy from the build tree shows up in that list as your terminal.

What happens on first run:

1. The program is not trusted yet, so it asks once. macOS shows **“Quadrant” would like to control this computer using accessibility features**, and the application is added to **System Settings → Privacy & Security → Accessibility**.
2. Switch **Quadrant** on there. (The same pane is one click away from the menu-bar icon: **Accessibility Settings…**.)
3. Nothing else. The program keeps checking, quietly, and registers the shortcuts as soon as the grant exists — **no restart is needed**, and the menu-bar icon is there the whole time.

The log says what is true for the running program, naming the application to look for:

```text
quadrant: Accessibility permission required: Grant Quadrant Accessibility: System Settings → Privacy & Security → Accessibility, then add or switch on Quadrant. Nothing here will work until you do.
quadrant: Accessibility status: NOT trusted (Quadrant, bundled application com.quadrant.app)
```

After the grant:

```text
quadrant: Accessibility permission granted; registering the shortcuts
quadrant: registered ⌃⌘; → row 1, column 1, 1x1
```

If **Quadrant** is not in that list, add it with the **+** button and ⌘⇧G to `/Applications/Quadrant.app`.

macOS does not keep that pane in one place: on macOS 27 this list is the one under **Privacy & Security → Device Control and Data Access**, and on earlier releases under **Privacy & Security → Accessibility**. It is the same list either way — the applications allowed to control other applications' windows.

**Two rows named Quadrant, or Quadrant and quadrant.** The list keeps every copy that has ever asked, and identifies them by name rather than by path, so a copy run from the build tree looks exactly like the installed one. `quadrant` in lower case is an executable with no bundle, which cannot be granted the permission under its own name: leave it off, or select it and remove it with **−**. Rows named **Quadrant** are bundle copies, and because they share a bundle identifier they go away together:

```sh
tccutil reset Accessibility com.quadrant.app
```

Let the installed application ask again afterwards. Nothing here can rescue a copy that is no longer installed: nothing points at `~/bin/quadrant` any more, and `rm ~/bin/quadrant` is the end of it.

A dialog that will not stay closed is the signature of a program that **exits and is restarted**: every start asks once, and launchd restarts a crashing job no more often than every ten seconds, so the prompt comes back every ten seconds. This program does not exit — a missing grant leaves it running with its menu-bar icon up, and it asks once per launch — so a log that repeats

```text
quadrant: Accessibility permission required: …
```

without the `quadrant running in the menu bar` line after it is that other program.

A rebuild produces a new ad-hoc signature, and macOS treats an application whose signature changed as a different application: the entry stays in the list but no longer covers the new build. Switch it off and on again, after removing the old row — or sign with a [Developer ID](#build), which keeps one identity across rebuilds. Running the program in the foreground from Terminal is different again: then the terminal holds the grant, and granting Terminal is what makes the program trusted.

## Menu-bar icon

Running the program puts one icon in the menu bar: a square divided into four quadrants, the app's only UI. Clicking it opens its menu.

| Item | What it does |
| --- | --- |
| About Quadrant | The standard macOS About panel: icon, name, version, credits. |
| Accessibility Settings… | Opens System Settings at the Accessibility pane, where the grant is switched on. |
| Quit Quadrant (⌘Q) | Quits, through the same path Ctrl+C takes. |

The icon appears whether or not the Accessibility grant has been given, so the menu — including **Quit Quadrant** — is always reachable.

The icon is a **template image**, so AppKit draws it itself: white on a dark menu bar, black on a light one, and again when either changes. Command-drag it to move it along the menu bar; macOS remembers where you left it.

The icon is drawn in code — there are no image assets in this project — and the same drawing is what the application icon is made of: `build.sh` renders it at every size an icon set needs (`cmd/icongen`, which calls the drawing the About panel uses) and hands the result to Apple's `iconutil`, so `Quadrant.app` carries an `AppIcon.icns` of the mark above it — white on the blue rounded square — for the Finder, the Dock, the Accessibility pane and the About panel. One drawing, so those four can never drift apart. Nothing else about the program needs the icon: if the session has no menu bar to put it in, the shortcuts still start and a line is logged instead.

Quitting from the menu is the same shutdown as Ctrl+C: the shortcut registrations are released and the configuration watcher stops before the process ends.

## Run / stop

Run in the foreground to inspect logs:

```sh
./dist/.build/Quadrant.app/Contents/MacOS/Quadrant
```

Then quit from the menu-bar icon's **Quit Quadrant**, or with Ctrl+C in that terminal. To start in the background:

```sh
mkdir -p ~/Library/Logs/quadrant
nohup ./dist/.build/Quadrant.app/Contents/MacOS/Quadrant > ~/Library/Logs/quadrant/quadrant.log 2>&1 &
```

Running from a terminal is also the way to exercise the program without granting anything: the terminal then holds the Accessibility grant if it has one, so a launcher that has permission makes the program trusted. The installed application is normally started instead, and not by hand:

```sh
open /Applications/Quadrant.app        # macOS owns the launch, and the log goes to the unified log
open --stdout ~/Library/Logs/quadrant/quadrant.log /Applications/Quadrant.app   # or into the same file
```

To stop a background instance:

```sh
pkill -x Quadrant
```

A successful reload is logged, so a background instance — `~/Library/Logs/quadrant/quadrant.log` above — shows configuration changes as they take effect:

```text
20:13:05 quadrant: configuration reloaded: /Users/you/.quadrant/configuration.json
```

The log file is opened in append mode and never truncated, so its lines accumulate across restarts. Nothing rotates it: `: > ~/Library/Logs/quadrant/quadrant.log` empties it in place, without restarting the program.

## Launch at login

[`launchd/com.quadrant.agent.plist`](launchd/com.quadrant.agent.plist) is a sample user LaunchAgent: nothing loads it for you, and [Install](#install) is the sequence with the substitution already made. It runs the executable **inside** `Quadrant.app` rather than a loose copy of the program, because that is what gives the process the bundle's identity and therefore the bundle's Accessibility grant.

Its paths spell out `/Users/you` rather than `~`: launchd expands neither `~` nor environment variables, and a redirect path containing `~` makes the job fail to spawn with exit code 78 (`EX_CONFIG`). Replace that prefix before loading it:

```sh
mkdir -p ~/Applications
cp -R dist/.build/Quadrant.app ~/Applications/              # the bundle the sample points at
sed -e "s|/Users/you|$HOME|g" launchd/com.quadrant.agent.plist \
  > ~/Library/LaunchAgents/com.quadrant.agent.plist
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.quadrant.agent.plist
```

Manage that job with:

```sh
launchctl print gui/$(id -u)/com.quadrant.agent         # status
launchctl kickstart -k gui/$(id -u)/com.quadrant.agent  # restart, e.g. after ./build.sh
launchctl bootout gui/$(id -u)/com.quadrant.agent       # stop and unload
```

| Key | Why the sample sets it |
| --- | --- |
| `RunAtLoad` | Starts the program when you log in. |
| `KeepAlive` → `SuccessfulExit: false` | Restarts it after a crash, but *not* after **Quit Quadrant** or `launchctl bootout`, which both exit 0. |
| `LimitLoadToSessionType` → `Aqua` | Keeps the job out of SSH and other non-graphical sessions, where there is no menu bar and no window server. |
| `ProcessType` → `Interactive` | Stops launchd from throttling it as a background job. |
| `StandardOutPath` / `StandardErrorPath` | launchd has no default log destination: without these keys the program's output is discarded, and it does not reach the unified log (`log show --predicate 'process == "Quadrant"'` stays empty). Everything quadrant logs goes to stderr, so `StandardErrorPath` is the key that matters. launchd creates the file and any missing parent directories for it. |

Adding **Quadrant.app** under **System Settings → General → Login Items** starts it too, but with no log redirection, no restart control and no session restriction.

Because launchd, not Terminal, is then the parent process, the grant has to be the application's own rather than one inherited from a terminal: grant **Quadrant**, the row for `/Applications/Quadrant.app` (see [Accessibility permission](#accessibility-permission)). Rebuilding with an ad-hoc signature gives the bundle a new identity and drops the grant, which is the reason to [sign with a Developer ID](#build) if you rebuild often.

## Project layout

```text
quadrant/
├── build.sh
├── install.sh                 # Copies the built bundle and handles its Accessibility grant
├── packaging/Info.plist       # Makes Quadrant.app an application, not a binary
├── go.mod
├── configuration.example.json
├── cmd/quadrant/main.go       # App setup, menu-bar extra, shortcut routing, AX placement, reload
├── cmd/quadrant/window.go     # Which window to move: the focused-window query and its fallback
├── cmd/icongen/main.go        # Writes the icon set the bundle's .icns is built from
├── internal/config/          # First-run config, validation, reload polling, tests
├── internal/geometry/        # Grid/region placement math and tests
├── internal/grant/           # Waiting for the Accessibility grant while running, tests
├── internal/icon/            # The sizes and file names an .iconset is made of, tests
├── internal/shortcut/        # Modifier IDs, key tables, binding parser, tests
├── internal/macos/           # NSScreen.visibleFrame, coordinate conversion, menu-bar extra, icon art
├── launchd/                  # Sample user LaunchAgent (not installed automatically)
└── dist/                     # Generated app: .build/Quadrant.app after ./build.sh
```

## Notes and limitations

- The Go modules are pinned in `go.mod`; `build.sh` generates `go.sum` with `go mod tidy` on first build.
- Permission and the bundle: the program needs the Accessibility grant to move other applications' windows, and macOS gives that grant to an application, which is what `packaging/Info.plist` makes it. Without the grant the program runs, keeps its menu-bar icon and logs what to switch on; the shortcuts are registered within two seconds of the grant being given, without a restart. It never exits for a missing grant: a program that did would be restarted by its LaunchAgent into asking again, which reads as a dialog that will not stay closed.
- The bundle is ad-hoc signed unless `QUADRANT_SIGN_IDENTITY` names a Developer ID. An ad-hoc signature changes on every rebuild, and macOS treats a changed signature as a different application, so the grant has to be given again after a rebuild — check it with `codesign --verify --strict` after copying the bundle into place, because a signature macOS rejects cannot hold a grant at all.
- Versions before the bundle installed a loose executable in `~/bin`; nothing points at it any more, but an old LaunchAgent may still be loaded. Stop it with `launchctl bootout gui/$(id -u)/com.quadrant.agent` before loading the current one.
- Only standard, movable application windows are expected to work. Full-screen apps, some dialog panels, or apps with fixed geometry may reject resizing.
- **Which window is moved.** The first query is macOS's own answer to “what has focus”: the system-wide Accessibility element's `kAXFocusedApplicationAttribute`, then that application's `kAXFocusedWindowAttribute`. Some applications answer the first one with no value at all (`AXError -25212`, *the attribute has no value*), so the window cannot be named that way; Chromium applications are where this was first seen, and nothing about the fallback is specific to them. Quadrant then lists the running applications, takes the one reporting itself frontmost, and moves its first standard, unminimized window ([`cmd/quadrant/window.go`](cmd/quadrant/window.go)). Non-window elements are skipped — the Finder answers that list with its desktop, an `AXScrollArea` covering every display — and a minimized window is used only when the application has nothing else, because it keeps a position and appears where it was put when restored. The fallback cannot tell *which* of several windows has focus, because that is what failed: it takes the first, and logs a notice naming the application and window it chose. When the fallback fails as well, the error carries both reasons (`…: the attribute has no value; frontmost application fallback: …`). Both routes are covered by tests that supply the Accessibility answers themselves, because whether an application answers the focused-application query depends on the application and cannot be arranged on demand.
- The Carbon API uses **key positions**; library registration interprets printed keyboard legends where possible for international keyboard layouts. Letter, digit, and named-key bindings follow that remapping. Symbol bindings use US/ANSI positions; on a non-US layout, prefer a named key or verify the symbol on the target layout.
- Application windows are changed *once per keypress*. There is no ongoing automatic tiling; polling the configuration file is the only periodic activity.
- Reloading is a 30-second poll, not a filesystem notification, so a change can take up to that long to apply. It also re-registers every shortcut rather than only the ones that changed.
- The menu-bar icon and the About panel are built on AppKit through `go-macos/objc`, so they need a session with a menu bar. A background instance with nobody logged in starts without the icon and says so in the log.
- This project was assembled and its platform-independent components tested in a Linux workspace. The macOS half has since been built and started on an Apple Silicon Mac (macOS 27): shortcut registration, the menu-bar icon, the About panel and the quit path were all exercised there — the menu through AppKit's own action dispatch rather than a pointer, and window placement not at all. The reload watcher is covered by tests that run anywhere, but the hot-key half of a reload — releasing combinations and claiming new ones while the app runs — has not been exercised on real hardware. Report compiler/runtime issues with the exact error output.
- The bundle and its ad-hoc signature, the launchd job running the executable from inside it without a grant, the shortcuts' registration when that job is trusted, the icon set and its .icns, and the shutdown path were exercised on that machine. Granting the permission in System Settings, the **Accessibility Settings…** menu item and window placement have not: the first needs a person at the machine, and the last needs an application whose window is allowed to move.

## Dependencies and license

The project is MIT-licensed. Its runtime libraries [`go-macos/hotkey`](https://github.com/go-macos/hotkey), [`go-macos/accessibility`](https://github.com/go-macos/accessibility), and [`go-macos/objc`](https://github.com/go-macos/objc) are BSD-3-Clause licensed.
