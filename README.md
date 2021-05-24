# USBGuard Watcher
Simple app that watches for usbguard events and requests action (allow/keep blocking) for each blocked device inserted. I was missing this so I made it. Makes my life easier. I appreciate any feedback.

## Depenpencies
- usbguard
- go

## Install
1. `go build -o usbguard-watcher main.go # build`
1. `sudo ln -s $(pwd)/usbguard-watcher /usr/local/bin/usbguard-watcher # install`

## Tips
- i3wm -> add this to i3config: `exec --no-startup-id usbguard-watcher`

## TODO
- [ ] make configurable (notify/choice binaries/options)
- [ ] add more docs
- [ ] add service file example
- [ ] add tests
- [ ] do some cleanup
- [ ] .github files
