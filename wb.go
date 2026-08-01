// xb-key-midi.go
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	midi "gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/midicatdrv"
)

const (
	evKey = 0x01
	evAbs = 0x03

	keyA = 30

	btnA = 304
	btnB = 305
	btnX = 307
	btnY = 308
)

var WLEDIPS = []string{"192.168.0.200", "192.168.0.201"}

var KeyMap = map[uint16]int{
	keyA: 88,
	btnA: 88,
	2:    184,
	3:    202,
	4:    202,
}

var LaunchpadCCMap = map[uint8]int{
	91: 1,
	92: 2,
	93: 3,
	94: 4,
}

var launchpadProgrammerMode = midi.SysEx([]byte{
	0x00, 0x20, 0x29, 0x02, 0x0D, 0x0E, 0x01,
})

type Event struct {
	Source string
	Code   uint16
	Value  int32
	Type   uint16
}

type LaunchpadState struct {
	mu         sync.Mutex
	outSender  func(midi.Message) error
	lastPressed uint8
}

// version is set at link time by build.sh (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	verbose := flag.Bool("verbose", false, "enable status logging")
	showVersion := flag.Bool("version", false, "print build version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	if *verbose {
		fmt.Println("Starting. Reading keyboard, Xbox controller, and MIDI input...")
	}

	events := make(chan Event, 64)
	var wg sync.WaitGroup

	keyboardDev, err := findKeyboardEvent()
	if err == nil {
		if *verbose {
			fmt.Printf("Using keyboard %s\n", keyboardDev)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			readDeviceLoop(keyboardDev, "keyboard", events, *verbose)
		}()
	} else if *verbose {
		fmt.Println("No keyboard device found")
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		readXboxDiscoveryLoop(events, *verbose)
	}()

	lp := &LaunchpadState{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		lp.readLaunchpadLoop(events, *verbose)
	}()

	go func() {
		wg.Wait()
		close(events)
	}()

	for ev := range events {
		handleEvent(ev, lp, *verbose)
	}
}

func handleEvent(ev Event, lp *LaunchpadState, verbose bool) {
	if ev.Source == "launchpad-midi" {
		if ev.Value == 0 {
			return
		}

		if preset, ok := LaunchpadCCMap[uint8(ev.Code)]; ok {
			fmt.Printf("[%s] CC %d -> preset %d\n", ev.Source, ev.Code, preset)
			sendWLEDPreset(preset)
		} else if verbose {
			fmt.Printf("[%s] unmapped MIDI code %d value %d\n", ev.Source, ev.Code, ev.Value)
		}
		return
	}

	switch ev.Type {
	case evKey:
		name := buttonName(ev.Code)
		switch ev.Value {
		case 1:
			fmt.Printf("[%s] %s pressed\n", ev.Source, name)
			if preset, ok := KeyMap[ev.Code]; ok {
				sendWLEDPreset(preset)
			} else if verbose {
				fmt.Printf("[%s] unmapped key/button: Code %d\n", ev.Source, ev.Code)
			}
		case 0:
			fmt.Printf("[%s] %s released\n", ev.Source, name)
		case 2:
			fmt.Printf("[%s] %s held\n", ev.Source, name)
		}
	case evAbs:
		fmt.Printf("[%s] ABS code %d = %d\n", ev.Source, ev.Code, ev.Value)
	}
}

func (lp *LaunchpadState) readLaunchpadLoop(out chan<- Event, verbose bool) {
	for {
		in, outSender, inName, outName, err := findLaunchpadPorts()
		if err != nil {
			if verbose {
				fmt.Println("No Launchpad MIDI ports found")
			}
			time.Sleep(2 * time.Second)
			continue
		}

		lp.mu.Lock()
		lp.outSender = outSender
		lp.mu.Unlock()

		if verbose {
			fmt.Printf("Using Launchpad MIDI in: %s\n", inName)
			fmt.Printf("Using Launchpad MIDI out: %s\n", outName)
		}

		if err := lp.sendProgrammerMode(verbose); err != nil && verbose {
			fmt.Printf("Failed to send programmer mode: %v\n", err)
		}

		stop, err := midi.ListenTo(in, func(msg midi.Message, timestampms int32) {
			var ch, num, val uint8

			if msg.GetControlChange(&ch, &num, &val) {
				out <- Event{
					Source: "launchpad-midi",
					Code:   uint16(num),
					Value:  int32(val),
				}
				if val > 0 {
					_ = lp.lightPad(num, 15, verbose)
				}
				return
			}

			if msg.GetNoteStart(&ch, &num, &val) {
				out <- Event{
					Source: "launchpad-midi",
					Code:   uint16(num),
					Value:  int32(val),
				}
				if val > 0 {
					_ = lp.lightPad(num, 15, verbose)
				}
				return
			}
		})

		if err != nil {
			if verbose {
				fmt.Printf("MIDI listen error: %v\n", err)
			}
			time.Sleep(2 * time.Second)
			continue
		}

		_ = stop
		time.Sleep(1 * time.Second)
	}
}

func (lp *LaunchpadState) sendProgrammerMode(verbose bool) error {
	lp.mu.Lock()
	sender := lp.outSender
	lp.mu.Unlock()

	if sender == nil {
		return fmt.Errorf("no Launchpad MIDI out sender")
	}

	if verbose {
		fmt.Println("Sending Launchpad Programmer Mode SysEx")
	}

	return sender(launchpadProgrammerMode)
}

func (lp *LaunchpadState) lightPad(note uint8, velocity uint8, verbose bool) error {
	lp.mu.Lock()
	sender := lp.outSender
	lp.lastPressed = note
	lp.mu.Unlock()

	if sender == nil {
		return nil
	}

	if verbose {
		fmt.Printf("Lighting pad note=%d velocity=%d\n", note, velocity)
	}

	return sender(midi.NoteOn(0, note, velocity))
}

func findLaunchpadPorts() (drivers.In, func(midi.Message) error, string, string, error) {
	in, err := midi.FindInPort("Launchpad")
	if err != nil {
		return nil, nil, "", "", err
	}

	outPort, err := midi.FindOutPort("Launchpad")
	if err != nil {
		return nil, nil, "", "", err
	}

	sender, err := midi.SendTo(outPort)
	if err != nil {
		return nil, nil, "", "", err
	}

	return in, sender, in.String(), outPort.String(), nil
}

func readDeviceLoop(path, source string, out chan<- Event, verbose bool) {
	for {
		if err := readInputDevice(path, source, out); err != nil {
			if verbose {
				fmt.Printf("%s disconnected or unavailable\n", source)
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func readXboxDiscoveryLoop(out chan<- Event, verbose bool) {
	for {
		dev, err := findXboxControllerEvent()
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		if verbose {
			fmt.Printf("Using xbox controller %s\n", dev)
		}
		if err := readInputDevice(dev, "xbox", out); err != nil && verbose {
			fmt.Println("controller disconnected")
		}
		time.Sleep(1 * time.Second)
	}
}

func readInputDevice(path, source string, out chan<- Event) error {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 24)

	fmt.Printf("Listening on %s: %s\n", source, path)

	for {
		_, err := io.ReadFull(f, buf)
		if err != nil {
			return err
		}

		typ := binary.LittleEndian.Uint16(buf[16:18])
		code := binary.LittleEndian.Uint16(buf[18:20])
		val := int32(binary.LittleEndian.Uint32(buf[20:24]))

		out <- Event{Source: source, Type: typ, Code: code, Value: val}
	}
}

func sendWLEDPreset(presetID int) {
	payload := map[string]int{"ps": presetID}
	body, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf(" -> Failed to encode payload: %v\n", err)
		return
	}

	fmt.Printf("Sending Preset %d to all controllers...\n", presetID)

	client := &http.Client{Timeout: 500 * time.Millisecond}

	for _, ip := range WLEDIPS {
		url := fmt.Sprintf("http://%s/json/state", ip)
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			fmt.Printf(" -> Failed to build request for %s: %v\n", ip, err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf(" -> Failed to reach %s: %v\n", ip, err)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			fmt.Printf(" -> Success: %s\n", ip)
		} else {
			fmt.Printf(" -> Error from %s: Status code %d\n", ip, resp.StatusCode)
		}
	}
}

func findKeyboardEvent() (string, error) {
	f, err := os.Open("/proc/bus/input/devices")
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var block []string

	flush := func(lines []string) (string, bool) {
		if len(lines) == 0 {
			return "", false
		}

		text := strings.Join(lines, "\n")
		lower := strings.ToLower(text)

		if !strings.Contains(lower, "keyboard") {
			return "", false
		}

		re := regexp.MustCompile(`Handlers=.*\b(event[0-9]+)\b`)
		m := re.FindStringSubmatch(text)
		if len(m) == 2 {
			return filepath.Join("/dev/input", m[1]), true
		}
		return "", false
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if dev, ok := flush(block); ok {
				return dev, nil
			}
			block = block[:0]
			continue
		}
		block = append(block, line)
	}

	if dev, ok := flush(block); ok {
		return dev, nil
	}
	return "", scanner.Err()
}

func findXboxControllerEvent() (string, error) {
	f, err := os.Open("/proc/bus/input/devices")
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var block []string

	flush := func(lines []string) (string, bool) {
		if len(lines) == 0 {
			return "", false
		}

		text := strings.Join(lines, "\n")
		lower := strings.ToLower(text)

		if !strings.Contains(lower, "xbox") &&
			!strings.Contains(lower, "x-box") &&
			!strings.Contains(lower, "360") &&
			!strings.Contains(lower, "microsoft") {
			return "", false
		}

		re := regexp.MustCompile(`Handlers=.*\b(event[0-9]+)\b`)
		m := re.FindStringSubmatch(text)
		if len(m) == 2 {
			return filepath.Join("/dev/input", m[1]), true
		}
		return "", false
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if dev, ok := flush(block); ok {
				return dev, nil
			}
			block = block[:0]
			continue
		}
		block = append(block, line)
	}

	if dev, ok := flush(block); ok {
		return dev, nil
	}
	return "", scanner.Err()
}

func buttonName(code uint16) string {
	switch code {
	case keyA:
		return "KEY_A"
	case btnA:
		return "A"
	case btnB:
		return "B"
	case btnX:
		return "X"
	case btnY:
		return "Y"
	default:
		return fmt.Sprintf("CODE_%d", code)
	}
}

