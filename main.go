package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"text/template"
)

var notifyBin = "notify-send"
var choiceBin = "rofi"
var choiceStdin = "yes\nno"
var choiceAffirmative = "yes"
var choiceArgs = []string{"-dmenu", "-p", "Allow {{ .Name }}?"}

type Event struct {
	ID         int
	Type       string
	Target     string
	DeviceRule string
}

func (e *Event) Set(key, value string) {
	switch key {
	case "event":
		e.Type = value
		break
	case "target":
		e.Target = value
		break
	case "device_rule":
		e.DeviceRule = value
		break
	}
}

type PolicyChange struct {
	ID         int
	TargetOld  string
	TargetNew  string
	DeviceRule string
}

func (p *PolicyChange) Set(key, value string) {
	switch key {
	case "target_old":
		p.TargetOld = value
		break
	case "target_new":
		p.TargetNew = value
		break
	case "device_rule":
		p.DeviceRule = value
		break
	}
}

func watch(
	stdoutChan chan []byte,
	eventChan chan *Event,
	policyChanges chan *PolicyChange,
	exit chan bool,
) {
	cmd := exec.Command("usbguard", "watch")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		r := bufio.NewReader(stdout)
		i := 0
		isStart := true
		var event *Event
		var policy *PolicyChange
		for {
			line, _, err := r.ReadLine()
			if err != nil {
				log.Printf("readline err: %#v\n", err)
				break
			}
			if i == 0 {
				i++
				if bytes.Equal(line, []byte("[IPC] Connected")) {
					log.Println(string(line))
					continue
				} else {
					panic(string(line))
				}
			}
			i++

			isStart = bytes.HasPrefix(line, []byte("[device]"))
			if isStart {
				parts := bytes.SplitN(line, []byte(" "), 3)
				idStr := string(bytes.Split(parts[2], []byte("="))[1])
				id, _ := strconv.Atoi(idStr)
				if bytes.Equal(parts[1], []byte("PresenceChanged:")) {
					event = &Event{ID: id}
					policy = nil
				} else {
					event = nil
					policy = &PolicyChange{ID: id}
				}
			} else {
				parts := bytes.SplitN(line, []byte(" "), 2)
				kv := bytes.SplitN(parts[1], []byte("="), 2)
				key := string(kv[0])
				val := string(kv[1])
				isLast := key == "device_rule"
				if event != nil {
					event.Set(key, val)
					if isLast {
						eventChan <- event
					}
				} else if policy != nil {
					policy.Set(key, val)
					if isLast {
						policyChanges <- policy
					}
				}

			}

			stdoutChan <- line
		}
	}()

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	if err := cmd.Wait(); err != nil {
		log.Fatal(err)
	}
	exit <- true
}

func main() {
	// [IPC] Connected
	// [device] PresenceChanged: id=1337
	//  event=Remove
	//  target=block
	//  device_rule=block id dead:beef serial "000000000" name "Wireless " hash "31337" parent-hash "foobar" via-port "2-1" with-interface ff:ff:ff with-connect-type "hotplug"
	// [device] PresenceChanged: id=1337
	//  event=Insert
	//  target=block
	//  device_rule=block id dead:beef serial "000000000" name "Wireless " hash "31337" parent-hash "foobar" via-port "2-1" with-interface ff:ff:ff with-connect-type "hotplug"
	// [device] PolicyChanged: id=1337
	//  target_old=block
	//  target_new=block
	//  device_rule=block id dead:beef serial "000000000" name "Wireless " hash "31337" parent-hash "foobar" via-port "2-1" with-interface ff:ff:ff with-connect-type "hotplug"
	//  rule_id=313373

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	exit := make(chan bool)
	stdout := make(chan []byte, 10)
	events := make(chan *Event, 10)
	policyChanges := make(chan *PolicyChange, 10)
	go watch(stdout, events, policyChanges, exit)
	for {
		select {
		case event := <-events:
			log.Printf("event: %#v\n", event)
			handleEvent(event)
			break
		case policy := <-policyChanges:
			log.Printf("policy: %#v\n", policy)
			handlePolicyChange(policy)
			break
		case line := <-stdout:
			log.Printf("line: %#v\n", string(line))
			break
		case <-exit:
		case <-sigChan:
			log.Println("Exiting")
			return
		}
	}
}

func shellExec(stdin, app string, args ...string) (string, error) {
	cmd := exec.Command(app, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return out.String(), err
	}

	return out.String(), nil
}

// name \" SanDisk 3.2Gen1\"
var reDeviceName = regexp.MustCompile("name \"([^\"]*)\"")

func getDeviceNameFromRule(deviceRule string) string {
	matches := reDeviceName.FindStringSubmatch(deviceRule)
	if len(matches) < 2 {
		return ""
	}

	return strings.Replace(matches[1], " ", "", 1)
}

func allowDevice(id int) {
	stdout, err := shellExec("", "usbguard", "allow-device", fmt.Sprintf("%d", id))
	log.Printf("allow stdout: %s\n", stdout)
	log.Printf("allow err: %#v\n", err)
}

func notify(msg string) {
	stdout, err := shellExec("", notifyBin, msg)
	log.Printf("notify stdout: %s\n", stdout)
	log.Printf("notify err: %#v\n", err)
}

func parseTemplateStringWithDeviceName(tplStr, name string) string {
	tmpl, err := template.New("template").Parse(tplStr)
	if err != nil {
		return tplStr
	}

	type Ctx struct {
		Name string
	}
	ctx := Ctx{name}

	var out bytes.Buffer
	err = tmpl.Execute(&out, ctx)
	if err != nil {
		return tplStr
	}

	return out.String()
}

func handleEvent(event *Event) {
	if event.Type != "Insert" {
		return
	}

	name := getDeviceNameFromRule(event.DeviceRule)
	log.Printf("name: %s\n", name)

	choiceStdin = parseTemplateStringWithDeviceName(choiceStdin, name)
	for i, choice := range choiceArgs {
		choiceArgs[i] = parseTemplateStringWithDeviceName(choice, name)
	}

	stdout, err := shellExec(choiceStdin, choiceBin, choiceArgs...)
	choice := strings.Replace(stdout, "\n", "", 1)
	log.Printf("choice: [%s]\n", choice)
	log.Printf("err: [%#v]\n", err)

	if choice != choiceAffirmative {
		return
	}

	allowDevice(event.ID)
}

func handlePolicyChange(policy *PolicyChange) {
	if policy.TargetOld == policy.TargetNew {
		return
	}

	name := getDeviceNameFromRule(policy.DeviceRule)
	log.Printf("name: %s\n", name)

	msg := fmt.Sprintf("USB Policy changed for device [%s]: %s -> %s",
		name,
		policy.TargetOld,
		policy.TargetNew,
	)

	notify(msg)
}
