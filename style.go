package main

import "github.com/nsf/termbox-go"

/*
Style Defines the colors for the terminal display, basically
*/
type Style struct {
	defaultBg termbox.Attribute
	defaultFg termbox.Attribute
	titleFg   termbox.Attribute
	titleBg   termbox.Attribute
	cursorFg  termbox.Attribute
	cursorBg  termbox.Attribute
}

func defaultStyle() Style {
	var style Style
	style.defaultBg = termbox.ColorDefault
	style.defaultFg = termbox.ColorDefault
	style.titleFg = termbox.ColorWhite
	style.titleBg = termbox.ColorBlack
	style.cursorFg = termbox.ColorWhite
	style.cursorBg = termbox.ColorBlack

	return style
}
