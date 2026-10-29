package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Keys map[string][]string

func DefaultKeys() Keys {
	return Keys{
		"down": {"j", "down"}, "up": {"k", "up"}, "left": {"ctrl+h"}, "right": {"ctrl+l"}, "panel_down": {"ctrl+j"}, "panel_up": {"ctrl+k"}, "focus_next": {"tab"}, "focus_previous": {"shift+tab"},
		"top": {"home"}, "bottom": {"G", "end"}, "page_up": {"ctrl+u", "pgup"}, "page_down": {"ctrl+d", "pgdown"},
		"open": {"l", "right"}, "activate": {"enter"}, "back": {"h", "left", "esc", "backspace"}, "filter": {"/"}, "query": {"ctrl+f"}, "scope": {"S"},
		"category_next": {"."}, "category_previous": {","}, "toggle": {"space"}, "stop": {"x"}, "next": {"]"}, "previous": {"["},
		"seek_forward": {">"}, "seek_backward": {"<"}, "volume_up": {"="}, "volume_down": {"-"}, "repeat": {"r"}, "shuffle": {"z"},
		"enqueue": {"a"}, "play_next": {"A"}, "play_selection": {"p"}, "remove": {"d"}, "clear": {"C"}, "queue_up": {"K"}, "queue_down": {"J"}, "related": {"o"}, "credits": {"c"},
		"resize_up": {"ctrl+shift+k"}, "resize_down": {"ctrl+shift+j"}, "resize_left": {"ctrl+shift+h"}, "resize_right": {"ctrl+shift+l"},
		"layout_reset": {"ctrl+="}, "zen": {"ctrl++", "ctrl+shift++", "ctrl+shift+=", "ctrl+-", "ctrl+_", "ctrl+shift+_", "ctrl+shift+-"},
		"layout_next": {"+", "shift+="}, "layout_previous": {"_", "shift+-"}, "logs": {"@"}, "quality": {"b"}, "login": {"L"}, "refresh": {"R"}, "help": {"?"}, "leave": {"q", "Q", "ctrl+c"}, "command": {":"},
	}
}

func LoadKeys(dir string) (Keys, error) {
	keys := DefaultKeys()
	b, err := os.ReadFile(filepath.Join(dir, "keys.json"))
	if os.IsNotExist(err) {
		return keys, nil
	}
	if err != nil {
		return nil, err
	}
	var custom Keys
	if err = json.Unmarshal(b, &custom); err != nil {
		return nil, fmt.Errorf("keys.json: %w", err)
	}
	for action, value := range custom {
		if _, ok := keys[action]; !ok {
			return nil, fmt.Errorf("unknown key action %q", action)
		}
		keys[action] = value
	}
	seen := map[string]string{}
	for action, list := range keys {
		for _, key := range list {
			if old, ok := seen[key]; ok && old != action {
				return nil, fmt.Errorf("key %q is assigned to both %s and %s", key, old, action)
			}
			seen[key] = action
		}
	}
	return keys, nil
}

func (k Keys) Action(key string) string {
	for action, list := range k {
		for _, candidate := range list {
			if candidate == key {
				return action
			}
		}
	}
	return ""
}
