package main

import (
	"fmt"
	"github.com/br0xen/termbox-util"
	"github.com/nsf/termbox-go"
	"strings"
	"time"
)

type ViewPort struct {
	bytes_per_row  int
	number_of_rows int
	first_row      int
}

type BrowserScreen struct {
	db              *BoltDB
	view_port       ViewPort
	queued_command  string
	current_path    []string
	current_type    int
	message         string
	mode            BrowserMode
	input_modal     *termbox_util.InputModal
	confirm_modal   *termbox_util.ConfirmModal
	message_timeout time.Duration
	message_time    time.Time
}

type BrowserMode int

const (
	MODE_BROWSE          = 16  // 0000 0001 0000
	MODE_CHANGE          = 32  // 0000 0010 0000
	MODE_CHANGE_KEY      = 33  // 0000 0010 0001
	MODE_CHANGE_VAL      = 34  // 0000 0010 0010
	MODE_INSERT          = 64  // 0000 0100 0000
	MODE_INSERT_BUCKET   = 65  // 0000 0100 0001
	MODE_INSERT_PAIR     = 68  // 0000 0100 0100
	MODE_INSERT_PAIR_KEY = 69  // 0000 0100 0101
	MODE_INSERT_PAIR_VAL = 70  // 0000 0100 0110
	MODE_DELETE          = 256 // 0001 0000 0000
	MODE_MOD_TO_PARENT   = 8   // 0000 0000 1000
)

type BoltType int

const (
	TYPE_BUCKET = iota
	TYPE_PAIR
)

func (screen *BrowserScreen) handleKeyEvent(event termbox.Event) int {
	if screen.mode == 0 {
		screen.mode = MODE_BROWSE
	}
	if screen.mode == MODE_BROWSE {
		return screen.handleBrowseKeyEvent(event)
	} else if screen.mode&MODE_CHANGE == MODE_CHANGE {
		return screen.handleInputKeyEvent(event)
	} else if screen.mode&MODE_INSERT == MODE_INSERT {
		return screen.handleInsertKeyEvent(event)
	} else if screen.mode == MODE_DELETE {
		return screen.handleDeleteKeyEvent(event)
	}
	return BROWSER_SCREEN_INDEX
}

func (screen *BrowserScreen) handleBrowseKeyEvent(event termbox.Event) int {
	if event.Ch == '?' {
		// About
		return ABOUT_SCREEN_INDEX

	} else if event.Ch == 'q' || event.Key == termbox.KeyEsc || event.Key == termbox.KeyCtrlC {
		// Quit
		return EXIT_SCREEN_INDEX

	} else if event.Ch == 'g' {
		// Jump to Beginning
		screen.current_path = screen.db.getNextVisiblePath(nil)

	} else if event.Ch == 'G' {
		// Jump to End
		screen.current_path = screen.db.getPrevVisiblePath(nil)

	} else if event.Key == termbox.KeyCtrlR {
		screen.refreshDatabase()

	} else if event.Key == termbox.KeyCtrlF {
		// Jump forward half a screen
		_, h := termbox.Size()
		half := h / 2
		screen.jumpCursorDown(half)

	} else if event.Key == termbox.KeyCtrlB {
		_, h := termbox.Size()
		half := h / 2
		screen.jumpCursorUp(half)

	} else if event.Ch == 'j' || event.Key == termbox.KeyArrowDown {
		screen.moveCursorDown()

	} else if event.Ch == 'k' || event.Key == termbox.KeyArrowUp {
		screen.moveCursorUp()

	} else if event.Ch == 'p' {
		// p creates a new pair at the current level
		screen.startInsertItem(TYPE_PAIR)
	} else if event.Ch == 'P' {
		// P creates a new pair at the parent level
		screen.startInsertItemAtParent(TYPE_PAIR)

	} else if event.Ch == 'b' {
		// b creates a new bucket at the current level
		screen.startInsertItem(TYPE_BUCKET)
	} else if event.Ch == 'B' {
		// B creates a new bucket at the parent level
		screen.startInsertItemAtParent(TYPE_BUCKET)

	} else if event.Ch == 'e' {
		b, p, _ := screen.db.getGenericFromPath(screen.current_path)
		if b != nil {
			screen.setMessage("Cannot edit a bucket, did you mean to (r)ename?")
		} else if p != nil {
			screen.startEditItem()
		}

	} else if event.Ch == 'r' {
		screen.startRenameItem()

	} else if event.Key == termbox.KeyEnter {
		b, p, _ := screen.db.getGenericFromPath(screen.current_path)
		if b != nil {
			screen.db.toggleOpenBucket(screen.current_path)
		} else if p != nil {
			screen.startEditItem()
		}

	} else if event.Ch == 'l' || event.Key == termbox.KeyArrowRight {
		b, p, _ := screen.db.getGenericFromPath(screen.current_path)
		// Select the current item
		if b != nil {
			screen.db.toggleOpenBucket(screen.current_path)
		} else if p != nil {
			screen.startEditItem()
		} else {
			screen.setMessage("Not sure what to do here...")
		}

	} else if event.Ch == 'h' || event.Key == termbox.KeyArrowLeft {
		// If we are _on_ a bucket that's open, close it
		b, _, e := screen.db.getGenericFromPath(screen.current_path)
		if e == nil && b != nil && b.expanded {
			screen.db.closeBucket(screen.current_path)
		} else {
			if len(screen.current_path) > 1 {
				parent_bucket, err := screen.db.getBucketFromPath(screen.current_path[:len(screen.current_path)-1])
				if err == nil {
					screen.db.closeBucket(parent_bucket.GetPath())
					// Figure out how far up we need to move the cursor
					screen.current_path = parent_bucket.GetPath()
				}
			} else {
				screen.db.closeBucket(screen.current_path)
			}
		}

	} else if event.Ch == 'D' {
		screen.startDeleteItem()
	}
	return BROWSER_SCREEN_INDEX
}

func (screen *BrowserScreen) handleInputKeyEvent(event termbox.Event) int {
	if event.Key == termbox.KeyEsc {
		screen.mode = MODE_BROWSE
		screen.input_modal.Clear()
	} else {
		screen.input_modal.HandleKeyPress(event)
		if screen.input_modal.IsDone() {
			b, p, _ := screen.db.getGenericFromPath(screen.current_path)
			if b != nil {
				if screen.mode == MODE_CHANGE_KEY {
					new_name := screen.input_modal.GetValue()
					if renameBucket(screen.current_path, new_name) != nil {
						screen.setMessage("Error renaming bucket.")
					} else {
						b.name = new_name
						screen.current_path[len(screen.current_path)-1] = b.name
						screen.setMessage("Bucket Renamed!")
						screen.refreshDatabase()
					}
				}
			} else if p != nil {
				if screen.mode == MODE_CHANGE_KEY {
					new_key := screen.input_modal.GetValue()
					if updatePairKey(screen.current_path, new_key) != nil {
						screen.setMessage("Error occurred updating Pair.")
					} else {
						p.key = new_key
						screen.current_path[len(screen.current_path)-1] = p.key
						screen.setMessage("Pair updated!")
						screen.refreshDatabase()
					}
				} else if screen.mode == MODE_CHANGE_VAL {
					new_val := screen.input_modal.GetValue()
					if updatePairValue(screen.current_path, new_val) != nil {
						screen.setMessage("Error occurred updating Pair.")
					} else {
						p.val = new_val
						screen.setMessage("Pair updated!")
						screen.refreshDatabase()
					}
				}
			}
			screen.mode = MODE_BROWSE
			screen.input_modal.Clear()
		}
	}
	return BROWSER_SCREEN_INDEX
}

func (screen *BrowserScreen) handleDeleteKeyEvent(event termbox.Event) int {
	screen.confirm_modal.HandleKeyPress(event)
	if screen.confirm_modal.IsDone() {
		if screen.confirm_modal.IsAccepted() {
			hold_next_path := screen.db.getNextVisiblePath(screen.current_path)
			hold_prev_path := screen.db.getPrevVisiblePath(screen.current_path)
			if deleteKey(screen.current_path) == nil {
				screen.refreshDatabase()
				// Move the current path endpoint appropriately
				//found_new_path := false
				if hold_next_path != nil {
					if len(hold_next_path) > 2 {
						if hold_next_path[len(hold_next_path)-2] == screen.current_path[len(screen.current_path)-2] {
							screen.current_path = hold_next_path
						} else if hold_prev_path != nil {
							screen.current_path = hold_prev_path
						} else {
							// Otherwise, go to the parent
							screen.current_path = screen.current_path[:(len(hold_next_path) - 2)]
						}
					} else {
						// Root bucket deleted, set to next
						screen.current_path = hold_next_path
					}
				} else if hold_prev_path != nil {
					screen.current_path = hold_prev_path
				} else {
					screen.current_path = screen.current_path[:0]
				}
			}
		}
		screen.mode = MODE_BROWSE
		screen.confirm_modal.Clear()
	}
	return BROWSER_SCREEN_INDEX
}

func (screen *BrowserScreen) handleInsertKeyEvent(event termbox.Event) int {
	if event.Key == termbox.KeyEsc {
		if len(screen.db.buckets) == 0 {
			return EXIT_SCREEN_INDEX
		} else {
			screen.mode = MODE_BROWSE
			screen.input_modal.Clear()
		}
	} else {
		screen.input_modal.HandleKeyPress(event)
		if screen.input_modal.IsDone() {
			new_val := screen.input_modal.GetValue()
			screen.input_modal.Clear()
			var insert_path []string
			if len(screen.current_path) > 0 {
				_, p, e := screen.db.getGenericFromPath(screen.current_path)
				if e != nil {
					screen.setMessage("Error Inserting new item. Invalid Path.")
				}
				insert_path = screen.current_path
				// where are we inserting?
				if p != nil {
					// If we're sitting on a pair, we have to go to it's parent
					screen.mode = screen.mode | MODE_MOD_TO_PARENT
				}
				if screen.mode&MODE_MOD_TO_PARENT == MODE_MOD_TO_PARENT {
					if len(screen.current_path) > 1 {
						insert_path = screen.current_path[:len(screen.current_path)-1]
					} else {
						insert_path = make([]string, 0)
					}
				}
			}

			parent_b, _, _ := screen.db.getGenericFromPath(insert_path)
			if screen.mode&MODE_INSERT_BUCKET == MODE_INSERT_BUCKET {
				err := insertBucket(insert_path, new_val)
				if err != nil {
					screen.setMessage(fmt.Sprintf("%s => %s", err, insert_path))
				} else {
					if parent_b != nil {
						parent_b.expanded = true
					}
				}
				screen.current_path = append(insert_path, new_val)

				screen.refreshDatabase()
				screen.mode = MODE_BROWSE
				screen.input_modal.Clear()
			} else if screen.mode&MODE_INSERT_PAIR == MODE_INSERT_PAIR {
				err := insertPair(insert_path, new_val, "")
				if err != nil {
					screen.setMessage(fmt.Sprintf("%s => %s", err, insert_path))
					screen.refreshDatabase()
					screen.mode = MODE_BROWSE
					screen.input_modal.Clear()
				} else {
					if parent_b != nil {
						parent_b.expanded = true
					}
					screen.current_path = append(insert_path, new_val)
					screen.refreshDatabase()
					screen.startEditItem()
				}
			}
		}
	}
	return BROWSER_SCREEN_INDEX
}

func (screen *BrowserScreen) jumpCursorUp(distance int) bool {
	// Jump up 'distance' lines
	vis_paths, err := screen.db.buildVisiblePathSlice()
	if err == nil {
		find_path := strings.Join(screen.current_path, "/")
		start_jump := false
		for i := range vis_paths {
			if vis_paths[len(vis_paths)-1-i] == find_path {
				start_jump = true
			}
			if start_jump {
				distance -= 1
				if distance == 0 {
					screen.current_path = strings.Split(vis_paths[len(vis_paths)-1-i], "/")
					break
				}
			}
		}
		if strings.Join(screen.current_path, "/") == find_path {
			screen.current_path = screen.db.getNextVisiblePath(nil)
		}
	}
	return true
}
func (screen *BrowserScreen) jumpCursorDown(distance int) bool {
	vis_paths, err := screen.db.buildVisiblePathSlice()
	if err == nil {
		find_path := strings.Join(screen.current_path, "/")
		start_jump := false
		for i := range vis_paths {
			if vis_paths[i] == find_path {
				start_jump = true
			}
			if start_jump {
				distance -= 1
				if distance == 0 {
					screen.current_path = strings.Split(vis_paths[i], "/")
					break
				}
			}
		}
		if strings.Join(screen.current_path, "/") == find_path {
			screen.current_path = screen.db.getPrevVisiblePath(nil)
		}
	}
	return true
}

func (screen *BrowserScreen) moveCursorUp() bool {
	new_path := screen.db.getPrevVisiblePath(screen.current_path)
	if new_path != nil {
		screen.current_path = new_path
		return true
	}
	return false
}
func (screen *BrowserScreen) moveCursorDown() bool {
	new_path := screen.db.getNextVisiblePath(screen.current_path)
	if new_path != nil {
		screen.current_path = new_path
		return true
	}
	return false
}

func (screen *BrowserScreen) performLayout() {}

func (screen *BrowserScreen) drawScreen(style Style) {
	if len(screen.db.buckets) == 0 && screen.mode&MODE_INSERT_BUCKET != MODE_INSERT_BUCKET {
		// Force a bucket insert
		screen.startInsertItemAtParent(TYPE_BUCKET)
	}
	if screen.message == "" {
		screen.setMessageWithTimeout("Press '?' for help", -1)
	}
	screen.drawLeftPane(style)
	screen.drawRightPane(style)
	screen.drawHeader(style)
	screen.drawFooter(style)

	if screen.input_modal != nil {
		screen.input_modal.Draw()
	}
	if screen.mode == MODE_DELETE {
		screen.confirm_modal.Draw()
	}
}

func (screen *BrowserScreen) drawHeader(style Style) {
	width, _ := termbox.Size()
	spaces := strings.Repeat(" ", (width / 2))
	termbox_util.DrawStringAtPoint(fmt.Sprintf("%s%s%s", spaces, PROGRAM_NAME, spaces), 0, 0, style.title_fg, style.title_bg)
}
func (screen *BrowserScreen) drawFooter(style Style) {
	if screen.message_timeout > 0 && time.Since(screen.message_time) > screen.message_timeout {
		screen.clearMessage()
	}
	_, height := termbox.Size()
	termbox_util.DrawStringAtPoint(screen.message, 0, height-1, style.default_fg, style.default_bg)
}
func (screen *BrowserScreen) drawLeftPane(style Style) {
	w, h := termbox.Size()
	if w > 80 {
		w = w / 2
	}
	screen.view_port.number_of_rows = h - 2

	termbox_util.FillWithChar('=', 0, 1, w, 1, style.default_fg, style.default_bg)
	y := 2
	screen.view_port.first_row = y
	if len(screen.current_path) == 0 {
		screen.current_path = screen.db.getNextVisiblePath(nil)
	}

	// So we know how much of the tree _wants_ to be visible
	// we only have screen.view_port.number_of_rows of space though
	cur_path_spot := 0
	vis_slice, err := screen.db.buildVisiblePathSlice()
	if err == nil {
		for i := range vis_slice {
			if strings.Join(screen.current_path, "/") == vis_slice[i] {
				cur_path_spot = i
			}
		}
	}

	tree_offset := 0
	max_cursor := screen.view_port.number_of_rows * 2 / 3
	if cur_path_spot > max_cursor {
		tree_offset = cur_path_spot - max_cursor
	}

	for i := range screen.db.buckets {
		// The drawBucket function returns how many lines it took up
		bkt_h := screen.drawBucket(&screen.db.buckets[i], style, (y - tree_offset))
		y += bkt_h
	}
}
func (screen *BrowserScreen) drawRightPane(style Style) {
	w, h := termbox.Size()
	if w > 80 {
		// Screen is wide enough, split it
		termbox_util.FillWithChar('=', 0, 1, w, 1, style.default_fg, style.default_bg)
		termbox_util.FillWithChar('|', (w / 2), screen.view_port.first_row-1, (w / 2), h, style.default_fg, style.default_bg)

		b, p, err := screen.db.getGenericFromPath(screen.current_path)
		if err == nil {
			start_x := (w / 2) + 2
			start_y := 2
			if b != nil {
				termbox_util.DrawStringAtPoint(fmt.Sprintf("Path: %s", strings.Join(b.GetPath(), "/")), start_x, start_y, style.default_fg, style.default_bg)
				termbox_util.DrawStringAtPoint(fmt.Sprintf("Buckets: %d", len(b.buckets)), start_x, start_y+1, style.default_fg, style.default_bg)
				termbox_util.DrawStringAtPoint(fmt.Sprintf("Pairs: %d", len(b.pairs)), start_x, start_y+2, style.default_fg, style.default_bg)
			} else if p != nil {
				termbox_util.DrawStringAtPoint(fmt.Sprintf("Path: %s", strings.Join(p.GetPath(), "/")), start_x, start_y, style.default_fg, style.default_bg)
				termbox_util.DrawStringAtPoint(fmt.Sprintf("Key: %s", p.key), start_x, start_y+1, style.default_fg, style.default_bg)
				termbox_util.DrawStringAtPoint(fmt.Sprintf("Value: %s", p.val), start_x, start_y+2, style.default_fg, style.default_bg)
			}
		}
	}
}

/* drawBucket
 * @bkt *BoltBucket - The bucket to draw
 * @style Style - The style to use
 * @w int - The Width of the lines
 * @y int - The Y position to start drawing
 * return - The number of lines used
 */
func (screen *BrowserScreen) drawBucket(bkt *BoltBucket, style Style, y int) int {
	w, _ := termbox.Size()
	if w > 80 {
		w = w / 2
	}
	used_lines := 0
	bucket_fg := style.default_fg
	bucket_bg := style.default_bg
	if comparePaths(screen.current_path, bkt.GetPath()) {
		bucket_fg = style.cursor_fg
		bucket_bg = style.cursor_bg
	}

	bkt_string := strings.Repeat(" ", len(bkt.GetPath())*2) //screen.db.getDepthFromPath(bkt.GetPath())*2)
	if bkt.expanded {
		bkt_string = bkt_string + "- " + bkt.name + " "
		bkt_string = fmt.Sprintf("%s%s", bkt_string, strings.Repeat(" ", (w-len(bkt_string))))

		termbox_util.DrawStringAtPoint(bkt_string, 0, (y + used_lines), bucket_fg, bucket_bg)
		used_lines += 1

		for i := range bkt.buckets {
			used_lines += screen.drawBucket(&bkt.buckets[i], style, y+used_lines)
		}
		for i := range bkt.pairs {
			used_lines += screen.drawPair(&bkt.pairs[i], style, y+used_lines)
		}
	} else {
		bkt_string = bkt_string + "+ " + bkt.name
		bkt_string = fmt.Sprintf("%s%s", bkt_string, strings.Repeat(" ", (w-len(bkt_string))))
		termbox_util.DrawStringAtPoint(bkt_string, 0, (y + used_lines), bucket_fg, bucket_bg)
		used_lines += 1
	}
	return used_lines
}

func (screen *BrowserScreen) drawPair(bp *BoltPair, style Style, y int) int {
	w, _ := termbox.Size()
	if w > 80 {
		w = w / 2
	}
	bucket_fg := style.default_fg
	bucket_bg := style.default_bg
	if comparePaths(screen.current_path, bp.GetPath()) {
		bucket_fg = style.cursor_fg
		bucket_bg = style.cursor_bg
	}

	pair_string := strings.Repeat(" ", len(bp.GetPath())*2) //screen.db.getDepthFromPath(bp.GetPath())*2)
	pair_string = fmt.Sprintf("%s%s: %s", pair_string, bp.key, bp.val)
	if w-len(pair_string) > 0 {
		pair_string = fmt.Sprintf("%s%s", pair_string, strings.Repeat(" ", (w-len(pair_string))))
	}
	termbox_util.DrawStringAtPoint(pair_string, 0, y, bucket_fg, bucket_bg)
	return 1
}

func (screen *BrowserScreen) startDeleteItem() bool {
	b, p, e := screen.db.getGenericFromPath(screen.current_path)
	if e == nil {
		w, h := termbox.Size()
		inp_w, inp_h := (w / 2), 6
		inp_x, inp_y := ((w / 2) - (inp_w / 2)), ((h / 2) - inp_h)
		mod := termbox_util.CreateConfirmModal("", inp_x, inp_y, inp_w, inp_h, termbox.ColorWhite, termbox.ColorBlack)
		if b != nil {
			mod.SetTitle(termbox_util.AlignText(fmt.Sprintf("Delete Bucket '%s'?", b.name), inp_w-1, termbox_util.ALIGN_CENTER))
		} else if p != nil {
			mod.SetTitle(termbox_util.AlignText(fmt.Sprintf("Delete Pair '%s'?", p.key), inp_w-1, termbox_util.ALIGN_CENTER))
		}
		mod.Show()
		mod.SetText(termbox_util.AlignText("This cannot be undone!", inp_w-1, termbox_util.ALIGN_CENTER))
		screen.confirm_modal = mod
		screen.mode = MODE_DELETE
		return true
	}
	return false
}

func (screen *BrowserScreen) startEditItem() bool {
	_, p, e := screen.db.getGenericFromPath(screen.current_path)
	if e == nil {
		w, h := termbox.Size()
		inp_w, inp_h := (w / 2), 6
		inp_x, inp_y := ((w / 2) - (inp_w / 2)), ((h / 2) - inp_h)
		mod := termbox_util.CreateInputModal("", inp_x, inp_y, inp_w, inp_h, termbox.ColorWhite, termbox.ColorBlack)
		if p != nil {
			mod.SetTitle(termbox_util.AlignText(fmt.Sprintf("Input new value for '%s'", p.key), inp_w, termbox_util.ALIGN_CENTER))
			mod.SetValue(p.val)
		}
		mod.Show()
		screen.input_modal = mod
		screen.mode = MODE_CHANGE_VAL
		return true
	}
	return false
}

func (screen *BrowserScreen) startRenameItem() bool {
	b, p, e := screen.db.getGenericFromPath(screen.current_path)
	if e == nil {
		w, h := termbox.Size()
		inp_w, inp_h := (w / 2), 6
		inp_x, inp_y := ((w / 2) - (inp_w / 2)), ((h / 2) - inp_h)
		mod := termbox_util.CreateInputModal("", inp_x, inp_y, inp_w, inp_h, termbox.ColorWhite, termbox.ColorBlack)
		if b != nil {
			mod.SetTitle(termbox_util.AlignText(fmt.Sprintf("Rename Bucket '%s' to:", b.name), inp_w, termbox_util.ALIGN_CENTER))
			mod.SetValue(b.name)
		} else if p != nil {
			mod.SetTitle(termbox_util.AlignText(fmt.Sprintf("Rename Key '%s' to:", p.key), inp_w, termbox_util.ALIGN_CENTER))
			mod.SetValue(p.key)
		}
		mod.Show()
		screen.input_modal = mod
		screen.mode = MODE_CHANGE_KEY
		return true
	}
	return false
}

func (screen *BrowserScreen) startInsertItemAtParent(tp BoltType) bool {
	w, h := termbox.Size()
	inp_w, inp_h := w-1, 7
	if w > 80 {
		inp_w, inp_h = (w / 2), 7
	}
	inp_x, inp_y := ((w / 2) - (inp_w / 2)), ((h / 2) - inp_h)
	mod := termbox_util.CreateInputModal("", inp_x, inp_y, inp_w, inp_h, termbox.ColorWhite, termbox.ColorBlack)
	screen.input_modal = mod
	if len(screen.current_path) <= 0 {
		// in the root directory
		if tp == TYPE_BUCKET {
			mod.SetTitle(termbox_util.AlignText("Create Root Bucket", inp_w, termbox_util.ALIGN_CENTER))
			screen.mode = MODE_INSERT_BUCKET | MODE_MOD_TO_PARENT
			mod.Show()
			return true
		}
	} else {
		var ins_path string
		_, p, e := screen.db.getGenericFromPath(screen.current_path[:len(screen.current_path)-1])
		if e == nil && p != nil {
			ins_path = strings.Join(screen.current_path[:len(screen.current_path)-2], "/") + "/"
		} else {
			ins_path = strings.Join(screen.current_path[:len(screen.current_path)-1], "/") + "/"
		}
		title_prfx := ""
		if tp == TYPE_BUCKET {
			title_prfx = "New Bucket: "
		} else if tp == TYPE_PAIR {
			title_prfx = "New Pair: "
		}
		title_text := title_prfx + ins_path
		if len(title_text) > inp_w {
			trunc_w := len(title_text) - inp_w
			title_text = title_prfx + "..." + ins_path[trunc_w+3:]
		}
		if tp == TYPE_BUCKET {
			mod.SetTitle(termbox_util.AlignText(title_text, inp_w, termbox_util.ALIGN_CENTER))
			screen.mode = MODE_INSERT_BUCKET | MODE_MOD_TO_PARENT
			mod.Show()
			return true
		} else if tp == TYPE_PAIR {
			mod.SetTitle(termbox_util.AlignText(title_text, inp_w, termbox_util.ALIGN_CENTER))
			mod.Show()
			screen.mode = MODE_INSERT_PAIR | MODE_MOD_TO_PARENT
			return true
		}
	}
	return false
}

func (screen *BrowserScreen) startInsertItem(tp BoltType) bool {
	w, h := termbox.Size()
	inp_w, inp_h := w-1, 7
	if w > 80 {
		inp_w, inp_h = (w / 2), 7
	}
	inp_x, inp_y := ((w / 2) - (inp_w / 2)), ((h / 2) - inp_h)
	mod := termbox_util.CreateInputModal("", inp_x, inp_y, inp_w, inp_h, termbox.ColorWhite, termbox.ColorBlack)
	screen.input_modal = mod
	var ins_path string
	_, p, e := screen.db.getGenericFromPath(screen.current_path)
	if e == nil && p != nil {
		ins_path = strings.Join(screen.current_path[:len(screen.current_path)-1], "/") + "/"
	} else {
		ins_path = strings.Join(screen.current_path, "/") + "/"
	}
	title_prfx := ""
	if tp == TYPE_BUCKET {
		title_prfx = "New Bucket: "
	} else if tp == TYPE_PAIR {
		title_prfx = "New Pair: "
	}
	title_text := title_prfx + ins_path
	if len(title_text) > inp_w {
		trunc_w := len(title_text) - inp_w
		title_text = title_prfx + "..." + ins_path[trunc_w+3:]
	}
	if tp == TYPE_BUCKET {
		mod.SetTitle(termbox_util.AlignText(title_text, inp_w, termbox_util.ALIGN_CENTER))
		screen.mode = MODE_INSERT_BUCKET
		mod.Show()
		return true
	} else if tp == TYPE_PAIR {
		mod.SetTitle(termbox_util.AlignText(title_text, inp_w, termbox_util.ALIGN_CENTER))
		mod.Show()
		screen.mode = MODE_INSERT_PAIR
		return true
	}
	return false
}

func (screen *BrowserScreen) setMessage(msg string) {
	screen.message = msg
	screen.message_time = time.Now()
	screen.message_timeout = time.Second * 2
}

/* setMessageWithTimeout lets you specify the timeout for the message
 * setting it to -1 means it won't timeout
 */
func (screen *BrowserScreen) setMessageWithTimeout(msg string, timeout time.Duration) {
	screen.message = msg
	screen.message_time = time.Now()
	screen.message_timeout = timeout
}

func (screen *BrowserScreen) clearMessage() {
	screen.message = ""
	screen.message_timeout = -1
}

func (screen *BrowserScreen) refreshDatabase() {
	shadowDB := screen.db
	screen.db = screen.db.refreshDatabase()
	screen.db.syncOpenBuckets(shadowDB)
}

func comparePaths(p1, p2 []string) bool {
	return strings.Join(p1, "/") == strings.Join(p2, "/")
}