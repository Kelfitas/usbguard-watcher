# USBGuard Watcher

## Install
1. `go build -o usbguard-watcher main.go # build`
1. `sudo ln -s $(pwd)/usbguard-watcher /usr/local/bin/usbguard-watcher # install`

## Tips
- i3wm -> add this to i3config: `exec --no-startup-id usbguard-watcher`
