# Clockwork Orange on Windows and macOS

Version 4 ships one zip per platform from the
[releases page](https://github.com/ushineko/clockwork-orange/releases). No
runtime to install: the programs are native binaries.

## Windows 10 and 11

`clockwork-orange-windows-amd64.zip` holds `clockwork-orange.exe` (the command
line and the timer engine) and `clockwork-orange-gui.exe` (the window). Unzip
them anywhere together; double-click either one to open the window.

The executables are not code-signed, so SmartScreen warns on first run: choose
**More info**, then **Run anyway**. Once per download.

The window changes the wallpaper on the interval in Settings while it is open
or in the tray. To start it with Windows, put a shortcut to
`clockwork-orange-gui.exe` in the Startup folder (`shell:startup`). There is no
service on Windows; the timer lives in the window.

Multi-monitor: one image per monitor, stitched into a single spanned wallpaper
that matches your monitor layout, set through the registry and
`SystemParametersInfo`. The lock screen is not changed on Windows.

Command line, from a terminal in the unzipped folder:

    .\clockwork-orange.exe -d C:\Wallpapers            # a random image once
    .\clockwork-orange.exe -d C:\Wallpapers -w 600     # every ten minutes
    .\clockwork-orange.exe --desktop                   # from the enabled plugins
    .\clockwork-orange.exe --self-test                 # check the environment

## macOS 13 and later

`Clockwork-Orange-macOS.zip` holds `Clockwork Orange.app`. Move it to
Applications. It is not notarised, so the first launch is right-click, **Open**,
then confirm.

The command line is inside the bundle at
`Clockwork Orange.app/Contents/MacOS/clockwork-orange`; the same flags as
above apply. Wallpapers are set per screen. The lock screen is not changed on
macOS.

## Configuration

`~/.config/clockwork-orange.yml` on both platforms (the same file the Linux
version uses), written by the window. The download history and blacklist are
in `~/.config/clockwork-orange/`. See the README for the keys.

## Building

    go build ./cmd/clockwork-orange           # no CGO needed
    go build ./cmd/clockwork-orange-gui       # needs a C compiler (MinGW on Windows, Xcode tools on macOS)

CI builds the release artifacts with `fyne package`, which embeds the icon and,
on Windows, sets the GUI subsystem so no console window opens.
