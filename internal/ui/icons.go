package ui

import "os"

type iconSet struct {
	list, listOpen, open, done, due, notes, sub, add, edit, warn string
	sync, ok, off, trash                                         string
	capL, capR                                                   string // rounded powerline caps
}

// Nerd Font glyphs (what yazi uses). GTASK_ICONS=0 falls back to plain unicode.
var nerdIcons = iconSet{
	list: "", listOpen: "", open: "", done: "", due: "",
	notes: "", sub: "", add: "", edit: "", warn: "",
	sync: "", ok: "", off: "", trash: "",
	capL: "", capR: "",
}

var plainIcons = iconSet{
	list: "▸", listOpen: "▾", open: "○", done: "✓", due: "◷",
	notes: "≡", sub: "↳", add: "+", edit: "✎", warn: "!",
	sync: "↻", ok: "●", off: "✗", trash: "✕",
	capL: "", capR: "",
}

var ic = func() iconSet {
	if v := os.Getenv("GTASK_ICONS"); v == "0" || v == "false" || v == "plain" {
		return plainIcons
	}
	return nerdIcons
}()
