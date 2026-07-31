// xbkey-working.go
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
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv"
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

// Launchpad Mini MK3 stock Programmer Mode CCs
var LaunchpadCCMap = map[uint8]int{
	91: 1,
	92: 2,
	93: 3,
	94: 4,
}

type Event struct {
	Source string
	Code   uint16
	Value  int32
	Type   uint16
}

func main() {
	verbose := flag.Bool("verbose", false, "enable status logging")
	flag.Parse()

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

	wg.Add(1)
	go func() {
		defer wg.Done()
		readLaunchpadMIDILoop(events, *verbose)
	}()

	go func() {
		wg.Wait()
		close(events)
	}()

	for ev := range events {
		handleEvent(ev)
	}
}

func handleEvent(ev Event) {
	if ev.Source == "launchpad-midi" {
		if ev.Value == 0 {
			return
		}
		if preset, ok := LaunchpadCCMap[uint8(ev.Code)]; ok {
			fmt.Printf("[%s] CC %d -> preset %d\n", ev.Source, ev.Code, preset)
			sendWLEDPreset(preset)
		} else {
			fmt.Printf("[%s] unmapped CC %d value %d\n", ev.Source, ev.Code, ev.Value)
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
			} else {
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

func readLaunchpadMIDILoop(out chan<- Event, verbose bool) {
	for {
		in, err := midi.FindInPort("Launchpad")
		if err != nil {
			if verbose {
				fmt.Println("No Launchpad MIDI input found")
			}
			time.Sleep(2 * time.Second)
			continue
		}

		if verbose {
			fmt.Printf("Using MIDI input: %s\n", in.String())
		}

		stop, err := midi.ListenTo(in, func(msg midi.Message, timestampms int32) {
			var ch, cc, val uint8
			if msg.GetControlChange(&ch, &cc, &val) {
				out <- Event{
					Source: "launchpad-midi",
					Code:   uint16(cc),
					Value:  int32(val),
				}
			}
		})

		if err != nil {
			if verbose {
				fmt.Printf("MIDI listen error: %v\n", err)
			}
			time.Sleep(2 * time.Second)
			continue
		}

		// blocks until disconnected
		_ = stop
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

