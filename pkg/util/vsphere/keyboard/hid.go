// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package keyboard

import (
	"fmt"
	"strings"

	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// hidUsagePageKeyboard is the USB HID usage page for the keyboard/keypad
// page (USB HID Usage Tables, page 0x07).
const hidUsagePageKeyboard = 7

// encodeUsbHidCode combines a USB HID keyboard/keypad usage ID with the
// keyboard usage page into the single int32 value vSphere's
// UsbScanCodeSpecKeyEvent.UsbHidCode expects: (usageID << 16) | usagePage.
func encodeUsbHidCode(usageID int32) int32 {
	return usageID<<16 | hidUsagePageKeyboard
}

// USB HID Usage Tables, page 0x07 (Keyboard/Keypad), for a US QWERTY layout.
const (
	hidA          int32 = 0x04
	hidB          int32 = 0x05
	hidC          int32 = 0x06
	hidD          int32 = 0x07
	hidE          int32 = 0x08
	hidF          int32 = 0x09
	hidG          int32 = 0x0A
	hidH          int32 = 0x0B
	hidI          int32 = 0x0C
	hidJ          int32 = 0x0D
	hidK          int32 = 0x0E
	hidL          int32 = 0x0F
	hidM          int32 = 0x10
	hidN          int32 = 0x11
	hidO          int32 = 0x12
	hidP          int32 = 0x13
	hidQ          int32 = 0x14
	hidR          int32 = 0x15
	hidS          int32 = 0x16
	hidT          int32 = 0x17
	hidU          int32 = 0x18
	hidV          int32 = 0x19
	hidW          int32 = 0x1A
	hidX          int32 = 0x1B
	hidY          int32 = 0x1C
	hidZ          int32 = 0x1D
	hid1          int32 = 0x1E
	hid2          int32 = 0x1F
	hid3          int32 = 0x20
	hid4          int32 = 0x21
	hid5          int32 = 0x22
	hid6          int32 = 0x23
	hid7          int32 = 0x24
	hid8          int32 = 0x25
	hid9          int32 = 0x26
	hid0          int32 = 0x27
	hidEnter      int32 = 0x28
	hidEscape     int32 = 0x29
	hidBackspace  int32 = 0x2A
	hidTab        int32 = 0x2B
	hidSpace      int32 = 0x2C
	hidMinus      int32 = 0x2D
	hidEqual      int32 = 0x2E
	hidLeftBrace  int32 = 0x2F
	hidRightBrace int32 = 0x30
	hidBackslash  int32 = 0x31
	hidSemicolon  int32 = 0x33
	hidApostrophe int32 = 0x34
	hidGrave      int32 = 0x35
	hidComma      int32 = 0x36
	hidPeriod     int32 = 0x37
	hidSlash      int32 = 0x38
	hidF1         int32 = 0x3A
	hidF2         int32 = 0x3B
	hidF3         int32 = 0x3C
	hidF4         int32 = 0x3D
	hidF5         int32 = 0x3E
	hidF6         int32 = 0x3F
	hidF7         int32 = 0x40
	hidF8         int32 = 0x41
	hidF9         int32 = 0x42
	hidF10        int32 = 0x43
	hidF11        int32 = 0x44
	hidF12        int32 = 0x45
	hidInsert     int32 = 0x49
	hidHome       int32 = 0x4A
	hidPageUp     int32 = 0x4B
	hidDelete     int32 = 0x4C
	hidEnd        int32 = 0x4D
	hidPageDown   int32 = 0x4E
	hidRightArrow int32 = 0x4F
	hidLeftArrow  int32 = 0x50
	hidDownArrow  int32 = 0x51
	hidUpArrow    int32 = 0x52
)

// literalKey describes the USB HID usage ID for a literal character on a US
// QWERTY keyboard, and whether typing it requires the shift modifier.
type literalKey struct {
	usageID int32
	shift   bool
}

// literalKeys maps every printable ASCII character this package supports to
// its USB HID usage ID and whether it requires the shift modifier on a US
// QWERTY keyboard.
var literalKeys = func() map[rune]literalKey {
	m := map[rune]literalKey{
		' ': {hidSpace, false},
		'-': {hidMinus, false}, '_': {hidMinus, true},
		'=': {hidEqual, false}, '+': {hidEqual, true},
		'[': {hidLeftBrace, false}, '{': {hidLeftBrace, true},
		']': {hidRightBrace, false}, '}': {hidRightBrace, true},
		'\\': {hidBackslash, false}, '|': {hidBackslash, true},
		';': {hidSemicolon, false}, ':': {hidSemicolon, true},
		'\'': {hidApostrophe, false}, '"': {hidApostrophe, true},
		'`': {hidGrave, false}, '~': {hidGrave, true},
		',': {hidComma, false}, '<': {hidComma, true},
		'.': {hidPeriod, false}, '>': {hidPeriod, true},
		'/': {hidSlash, false}, '?': {hidSlash, true},
		'1': {hid1, false}, '!': {hid1, true},
		'2': {hid2, false}, '@': {hid2, true},
		'3': {hid3, false}, '#': {hid3, true},
		'4': {hid4, false}, '$': {hid4, true},
		'5': {hid5, false}, '%': {hid5, true},
		'6': {hid6, false}, '^': {hid6, true},
		'7': {hid7, false}, '&': {hid7, true},
		'8': {hid8, false}, '*': {hid8, true},
		'9': {hid9, false}, '(': {hid9, true},
		'0': {hid0, false}, ')': {hid0, true},
	}

	letters := []int32{
		hidA, hidB, hidC, hidD, hidE, hidF, hidG, hidH, hidI, hidJ, hidK, hidL,
		hidM, hidN, hidO, hidP, hidQ, hidR, hidS, hidT, hidU, hidV, hidW, hidX,
		hidY, hidZ,
	}
	for i, usageID := range letters {
		lower := rune('a' + i)
		upper := rune('A' + i)
		m[lower] = literalKey{usageID, false}
		m[upper] = literalKey{usageID, true}
	}

	return m
}()

// specialKeys maps the lowercase, bracket-free name of a special key token
// (e.g. "esc", "f1", "leftarrow") to its USB HID usage ID.
var specialKeys = map[string]int32{
	"esc":      hidEscape,
	"enter":    hidEnter,
	"tab":      hidTab,
	"bs":       hidBackspace,
	"del":      hidDelete,
	"spacebar": hidSpace,
	"up":       hidUpArrow,
	"down":     hidDownArrow,
	"left":     hidLeftArrow,
	"right":    hidRightArrow,
	"f1":       hidF1,
	"f2":       hidF2,
	"f3":       hidF3,
	"f4":       hidF4,
	"f5":       hidF5,
	"f6":       hidF6,
	"f7":       hidF7,
	"f8":       hidF8,
	"f9":       hidF9,
	"f10":      hidF10,
	"f11":      hidF11,
	"f12":      hidF12,
	"insert":   hidInsert,
	"home":     hidHome,
	"pageup":   hidPageUp,
	"end":      hidEnd,
	"pagedown": hidPageDown,
}

// modifierNames maps the lowercase name used in <leftShiftOn>/<leftShiftOff>
// style tokens to the field it sets on a UsbScanCodeSpecModifierType. The
// setter is called with nil to release the modifier (clearing the field
// entirely, rather than leaving an explicit "false" that would otherwise be
// sent on every subsequent, unrelated keystroke) or &true to hold it.
var modifierNames = map[string]func(*vimtypes.UsbScanCodeSpecModifierType, *bool){
	"leftcontrol":  func(m *vimtypes.UsbScanCodeSpecModifierType, v *bool) { m.LeftControl = v },
	"leftshift":    func(m *vimtypes.UsbScanCodeSpecModifierType, v *bool) { m.LeftShift = v },
	"leftalt":      func(m *vimtypes.UsbScanCodeSpecModifierType, v *bool) { m.LeftAlt = v },
	"leftgui":      func(m *vimtypes.UsbScanCodeSpecModifierType, v *bool) { m.LeftGui = v },
	"rightcontrol": func(m *vimtypes.UsbScanCodeSpecModifierType, v *bool) { m.RightControl = v },
	"rightshift":   func(m *vimtypes.UsbScanCodeSpecModifierType, v *bool) { m.RightShift = v },
	"rightalt":     func(m *vimtypes.UsbScanCodeSpecModifierType, v *bool) { m.RightAlt = v },
	"rightgui":     func(m *vimtypes.UsbScanCodeSpecModifierType, v *bool) { m.RightGui = v },
}

// isModifierName reports whether name (lowercased) is a recognized modifier
// key name, e.g. "leftShift".
func isModifierName(name string) bool {
	_, ok := modifierNames[strings.ToLower(name)]
	return ok
}

// applyModifier holds down (held=true) or releases (held=false) the
// modifier key corresponding to name on m.
func applyModifier(m *vimtypes.UsbScanCodeSpecModifierType, name string, held bool) error {
	fn, ok := modifierNames[strings.ToLower(name)]
	if !ok {
		return fmt.Errorf("unknown modifier key %q", name)
	}

	var v *bool
	if held {
		t := true
		v = &t
	}
	fn(m, v)
	return nil
}

// mergeModifiers returns a new UsbScanCodeSpecModifierType with every
// non-nil field from base and override set, with override's fields taking
// precedence. Returns nil if the result would have no fields set.
func mergeModifiers(base, override *vimtypes.UsbScanCodeSpecModifierType) *vimtypes.UsbScanCodeSpecModifierType {
	if base == nil && override == nil {
		return nil
	}

	out := &vimtypes.UsbScanCodeSpecModifierType{}
	for _, m := range []*vimtypes.UsbScanCodeSpecModifierType{base, override} {
		if m == nil {
			continue
		}
		if m.LeftControl != nil {
			out.LeftControl = m.LeftControl
		}
		if m.LeftShift != nil {
			out.LeftShift = m.LeftShift
		}
		if m.LeftAlt != nil {
			out.LeftAlt = m.LeftAlt
		}
		if m.LeftGui != nil {
			out.LeftGui = m.LeftGui
		}
		if m.RightControl != nil {
			out.RightControl = m.RightControl
		}
		if m.RightShift != nil {
			out.RightShift = m.RightShift
		}
		if m.RightAlt != nil {
			out.RightAlt = m.RightAlt
		}
		if m.RightGui != nil {
			out.RightGui = m.RightGui
		}
	}

	if *out == (vimtypes.UsbScanCodeSpecModifierType{}) {
		return nil
	}

	return out
}

// literalKeyEvent returns the key event needed to type the literal
// character r, combining the shift modifier the character itself requires
// (if any) with heldModifiers (set by <xOn>/<xOff> tokens).
func literalKeyEvent(r rune, heldModifiers *vimtypes.UsbScanCodeSpecModifierType) (vimtypes.UsbScanCodeSpecKeyEvent, error) {
	lk, ok := literalKeys[r]
	if !ok {
		return vimtypes.UsbScanCodeSpecKeyEvent{}, fmt.Errorf("unsupported character %q", r)
	}

	var shift *vimtypes.UsbScanCodeSpecModifierType
	if lk.shift {
		t := true
		shift = &vimtypes.UsbScanCodeSpecModifierType{LeftShift: &t}
	}

	return vimtypes.UsbScanCodeSpecKeyEvent{
		UsbHidCode: encodeUsbHidCode(lk.usageID),
		Modifiers:  mergeModifiers(heldModifiers, shift),
	}, nil
}

// specialKeyEvent returns the key event for the named special key (e.g.
// "esc", "f1"), combined with heldModifiers.
func specialKeyEvent(name string, heldModifiers *vimtypes.UsbScanCodeSpecModifierType) (vimtypes.UsbScanCodeSpecKeyEvent, error) {
	usageID, ok := specialKeys[strings.ToLower(name)]
	if !ok {
		return vimtypes.UsbScanCodeSpecKeyEvent{}, fmt.Errorf("unknown special key %q", name)
	}

	return vimtypes.UsbScanCodeSpecKeyEvent{
		UsbHidCode: encodeUsbHidCode(usageID),
		Modifiers:  mergeModifiers(heldModifiers, nil),
	}, nil
}
