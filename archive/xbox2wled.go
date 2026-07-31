package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	evKey = 0x01
	evAbs = 0x03
)

const (
	btnA         = 304
	btnB         = 305
	btnX         = 307
	btnY         = 308
	btnTL        = 310
	btnTR        = 311
	btnSelect    = 314
	btnStart     = 315
	btnMode      = 316
	btnThumbl    = 317
	btnThumbr    = 318
	btnDpadUp    = 544
	btnDpadDown  = 545
	btnDpadLeft  = 546
	btnDpadRight = 547
)

const (
	absLX = 0
	absLY = 1
	absLT = 2
	absRX = 3
	absRY = 4
	absRT = 5
)

type State struct {
	LX, LY, RX, RY int32
	LT, RT         int32
}

func main() {
	quiet := flag.Bool("quiet", false, "disable logging of LT, RT, and RY inputs")
	flag.Parse()

	fmt.Println("Starting. Plug in an Xbox controller anytime...")

	for {
		dev, err := findXboxControllerEvent()
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		fmt.Printf("Using %s\n", dev)

		f, err := os.OpenFile(dev, os.O_RDONLY, 0)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		if err := readLoop(f, *quiet); err != nil {
			fmt.Println("controller disconnected")
		}
		_ = f.Close()

		time.Sleep(1 * time.Second)
	}
}

func readLoop(f *os.File, quiet bool) error {
	buf := make([]byte, 24)
	var s State

	for {
		_, err := io.ReadFull(f, buf)
		if err != nil {
			return err
		}

		typ := binary.LittleEndian.Uint16(buf[16:18])
		code := binary.LittleEndian.Uint16(buf[18:20])
		val := int32(binary.LittleEndian.Uint32(buf[20:24]))

		switch typ {
		case evKey:
			name := buttonName(code)
			switch val {
			case 1:
				fmt.Printf("%s pressed\n", name)
			case 0:
				fmt.Printf("%s released\n", name)
			case 2:
				fmt.Printf("%s held\n", name)
			}

		case evAbs:
			switch code {
			case absLX:
				newVal := normalizeStick(val)
				if newVal != s.LX {
					s.LX = newVal
					fmt.Printf("LX = %d\n", s.LX)
				}
			case absLY:
				newVal := normalizeStick(val)
				if newVal != s.LY {
					s.LY = newVal
					fmt.Printf("LY = %d\n", s.LY)
				}
			case absRX:
				newVal := normalizeStick(val)
				if newVal != s.RX {
					s.RX = newVal
					fmt.Printf("RX = %d\n", s.RX)
				}
			case absRY:
				if quiet {
					continue
				}
				newVal := normalizeStick(val)
				if newVal != s.RY {
					s.RY = newVal
					fmt.Printf("RY = %d\n", s.RY)
				}
			case absLT:
				if quiet {
					continue
				}
				newVal := normalizeTrigger(val)
				if newVal != s.LT {
					s.LT = newVal
					fmt.Printf("LT = %d\n", s.LT)
				}
			case absRT:
				if quiet {
					continue
				}
				newVal := normalizeTrigger(val)
				if newVal != s.RT {
					s.RT = newVal
					fmt.Printf("RT = %d\n", s.RT)
				}
			}
		}
	}
}

func normalizeStick(raw int32) int32 {
	const deadzone = 4000
	const maxRange = 32767.0

	if raw > -deadzone && raw < deadzone {
		return 0
	}

	v := float64(raw) / maxRange * 100.0

	if v > 100 {
		v = 100
	}
	if v < -100 {
		v = -100
	}

	return int32(v)
}

func normalizeTrigger(raw int32) int32 {
	const deadzone = 5
	if raw <= deadzone {
		return 0
	}

	var maxRange float64 = 255
	if raw > 255 {
		maxRange = 1023
	}

	v := float64(raw-deadzone) / float64(maxRange-deadzone) * 100.0
	if v > 100 {
		v = 100
	}
	if v < 0 {
		v = 0
	}
	return int32(v)
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
	case btnA:
		return "A"
	case btnB:
		return "B"
	case btnX:
		return "X"
	case btnY:
		return "Y"
	case btnTL:
		return "LB"
	case btnTR:
		return "RB"
	case btnSelect:
		return "BACK"
	case btnStart:
		return "START"
	case btnMode:
		return "GUIDE"
	case btnThumbl:
		return "L3"
	case btnThumbr:
		return "R3"
	case btnDpadUp:
		return "DPAD_UP"
	case btnDpadDown:
		return "DPAD_DOWN"
	case btnDpadLeft:
		return "DPAD_LEFT"
	case btnDpadRight:
		return "DPAD_RIGHT"
	default:
		return fmt.Sprintf("CODE_%d", code)
	}
}

