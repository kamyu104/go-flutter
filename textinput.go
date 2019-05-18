package flutter

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/go-flutter-desktop/go-flutter/plugin"
	"github.com/go-gl/glfw/v3.2/glfw"
	"github.com/pkg/errors"
)

// Android KeyEvent constants from https://developer.android.com/reference/android/view/KeyEvent
const androidMetaStateShift = 1 << 0
const androidMetaStateAlt = 1 << 1
const androidMetaStateCtrl = 1 << 12
const androidMetaStateMeta = 1 << 16

const textinputChannelName = "flutter/textinput"
const keyEventChannelName = "flutter/keyevent"

// textinputPlugin implements flutter.Plugin and handles method calls to the
// flutter/platform channel.
type textinputPlugin struct {
	messenger   plugin.BinaryMessenger
	window      *glfw.Window
	textChannel *plugin.MethodChannel

	keyEventChannel *plugin.BasicMessageChannel

	keyboardLayout KeyboardShortcuts

	modifierKey           glfw.ModifierKey
	wordTravellerKey      glfw.ModifierKey
	wordTravellerKeyShift glfw.ModifierKey

	clientID        float64
	word            []rune
	selectionBase   int
	selectionExtent int
}

// all hardcoded because theres not pluggable renderer system.
var defaultTextinputPlugin = &textinputPlugin{}

var _ Plugin = &textinputPlugin{}     // compile-time type check
var _ PluginGLFW = &textinputPlugin{} // compile-time type check

func (p *textinputPlugin) InitPlugin(messenger plugin.BinaryMessenger) error {
	p.messenger = messenger

	// set modifier keys based on OS
	switch runtime.GOOS {
	case "darwin":
		p.modifierKey = glfw.ModSuper
		p.wordTravellerKey = glfw.ModAlt
		p.wordTravellerKeyShift = glfw.ModAlt | glfw.ModShift
	default:
		p.modifierKey = glfw.ModControl
		p.wordTravellerKey = glfw.ModControl
		p.wordTravellerKeyShift = glfw.ModControl | glfw.ModShift
	}

	return nil
}

func (p *textinputPlugin) InitPluginGLFW(window *glfw.Window) error {
	p.window = window
	p.textChannel = plugin.NewMethodChannel(p.messenger, textinputChannelName, plugin.JSONMethodCodec{})
	p.keyEventChannel = plugin.NewBasicMessageChannel(p.messenger, keyEventChannelName, plugin.JSONMessageCodec{})
	p.textChannel.HandleFuncSync("TextInput.setClient", p.handleSetClient)
	p.textChannel.HandleFuncSync("TextInput.clearClient", p.handleClearClient)
	p.textChannel.HandleFuncSync("TextInput.setEditingState", p.handleSetEditingState)

	return nil
}

func (p *textinputPlugin) handleSetClient(arguments interface{}) (reply interface{}, err error) {
	var args []interface{}
	err = json.Unmarshal(arguments.(json.RawMessage), &args)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode json arguments for handleSetClient")
	}
	p.clientID = args[0].(float64)
	return nil, nil
}

func (p *textinputPlugin) handleClearClient(arguments interface{}) (reply interface{}, err error) {
	p.clientID = 0
	return nil, nil
}

func (p *textinputPlugin) handleSetEditingState(arguments interface{}) (reply interface{}, err error) {
	if p.clientID == 0 {
		return nil, errors.New("cannot set editing state when no client is selected")
	}

	editingState := argsEditingState{}
	err = json.Unmarshal(arguments.(json.RawMessage), &editingState)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode json arguments for handleSetEditingState")
	}

	p.word = []rune(editingState.Text)
	p.selectionBase = editingState.SelectionBase
	p.selectionExtent = editingState.SelectionExtent
	return nil, nil
}

func (p *textinputPlugin) glfwCharCallback(w *glfw.Window, char rune) {
	if p.clientID == 0 {
		return
	}
	p.addChar([]rune{char})
}

func (p *textinputPlugin) glfwKeyCallback(window *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
	var modsIsModfifier = false
	var modsIsShift = false
	var modsIsWordModifierShift = false
	var modsIsWordModifier = false

	switch {
	case mods == p.wordTravellerKeyShift:
		modsIsWordModifierShift = true
	case mods == p.wordTravellerKey:
		modsIsWordModifier = true
	case mods == p.modifierKey:
		modsIsModfifier = true
	case mods == glfw.ModShift:
		modsIsShift = true
	}

	if key == glfw.KeyEscape && action == glfw.Press {
		_, err := defaultNavigationPlugin.channel.InvokeMethod("popRoute", nil)
		if err != nil {
			fmt.Printf("go-flutter: failed to pop route after escape key press: %v\n", err)
		}
		return
	}

	if action == glfw.Repeat || action == glfw.Press {
		if p.clientID == 0 {
			return
		}

		switch key {
		case glfw.KeyEnter:
			if mods == p.modifierKey {
				p.performAction("done")
			} else {
				p.addChar([]rune{'\n'})
				p.performAction("newline")
			}

		case glfw.KeyHome:
			p.MoveCursorHome(modsIsModfifier, modsIsShift, modsIsWordModifierShift, modsIsWordModifier)

		case glfw.KeyEnd:
			p.MoveCursorEnd(modsIsModfifier, modsIsShift, modsIsWordModifierShift, modsIsWordModifier)

		case glfw.KeyLeft:
			p.MoveCursorLeft(modsIsModfifier, modsIsShift, modsIsWordModifierShift, modsIsWordModifier)

		case glfw.KeyRight:
			p.MoveCursorRight(modsIsModfifier, modsIsShift, modsIsWordModifierShift, modsIsWordModifier)

		case glfw.KeyDelete:
			p.Delete(modsIsModfifier, modsIsShift, modsIsWordModifierShift, modsIsWordModifier)

		case glfw.KeyBackspace:
			p.Backspace(modsIsModfifier, modsIsShift, modsIsWordModifierShift, modsIsWordModifier)

		case p.keyboardLayout.SelectAll:
			if mods == p.modifierKey {
				p.SelectAll()
			}

		case p.keyboardLayout.Copy:
			if mods == p.modifierKey && p.isSelected() {
				_, _, selectedContent := p.GetSelectedText()
				window.SetClipboardString(selectedContent)
			}

		case p.keyboardLayout.Cut:
			if mods == p.modifierKey && p.isSelected() {
				_, _, selectedContent := p.GetSelectedText()
				window.SetClipboardString(selectedContent)
				p.RemoveSelectedText()
			}

		case p.keyboardLayout.Paste:
			if mods == p.modifierKey {
				var clpString, err = window.GetClipboardString()
				if err != nil {
					fmt.Printf("go-flutter: unable to get the clipboard content: %v\n", err)
					return
				}
				p.addChar([]rune(clpString))
			}
		}
	}

	// key events

	// TODO: Stop using the android keymap and translate the glfw keycode to the
	// platfom native one
	// BUG: the LogicalKeyboardKey isn't the right one
	// https://github.com/flutter/flutter/blob/1f2972c7b6a8503f7c6a5dfa180521a6f7efd472/packages/flutter/lib/src/services/raw_keyboard_android.dart#L116

	// MacOS example: flutter/engine/pull/8219
	// Linux/Windows Watch: google/flutter-desktop-embedding/issues/323
	var typeKey string
	if action == glfw.Release {
		typeKey = "keyup"
	} else if action == glfw.Press {
		typeKey = "keydown"
	} else {
		fmt.Printf("go-flutter: failed to send key event, action: %v\n", action)
		return
	}

	event := struct {
		KeyCode   int    `json:"keyCode"`
		Keymap    string `json:"keymap"`
		Type      string `json:"type"`
		MetaState int    `json:"metaState"`
	}{
		int(key), "android", typeKey,
		conditionalInt(mods&glfw.ModShift != 0, androidMetaStateShift) |
			conditionalInt(mods&glfw.ModAlt != 0, androidMetaStateAlt) |
			conditionalInt(mods&glfw.ModControl != 0, androidMetaStateCtrl) |
			conditionalInt(mods&glfw.ModSuper != 0, androidMetaStateMeta),
	}
	p.keyEventChannel.Send(event)

}

// Int returns val1 if condition, otherwise 0
func conditionalInt(condition bool, val1 int) int {
	if condition {
		return val1
	}
	return 0
}
