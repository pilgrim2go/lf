package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"

	"github.com/nsf/termbox-go"
)

type Win struct {
	w int
	h int
	x int
	y int
}

func newWin(w, h, x, y int) *Win {
	return &Win{w, h, x, y}
}

func (win *Win) renew(w, h, x, y int) {
	win.w = w
	win.h = h
	win.x = x
	win.y = y
}

func (win *Win) print(x, y int, fg, bg termbox.Attribute, s string) {
	off := x
	for _, c := range s {
		if x >= win.w {
			break
		}

		termbox.SetCell(win.x+x, win.y+y, c, fg, bg)

		if c == '\t' {
			x += gOpts.tabstop - (x-off)%gOpts.tabstop
		} else {
			x++
		}
	}
}

func (win *Win) printf(x, y int, fg, bg termbox.Attribute, format string, a ...interface{}) {
	win.print(x, y, fg, bg, fmt.Sprintf(format, a...))
}

func (win *Win) printl(x, y int, fg, bg termbox.Attribute, s string) {
	win.printf(x, y, fg, bg, "%s%*s", s, win.w-len(s), "")
}

func (win *Win) printd(dir *Dir, marks map[string]bool) {
	if win.w < 3 {
		return
	}

	fg, bg := termbox.ColorDefault, termbox.ColorDefault

	if len(dir.fi) == 0 {
		fg = termbox.AttrBold
		win.print(0, 0, fg, bg, "empty")
		return
	}

	maxind := len(dir.fi) - 1

	beg := max(dir.ind-dir.pos, 0)
	end := min(beg+win.h, maxind+1)

	for i, f := range dir.fi[beg:end] {
		switch {
		case f.Mode().IsRegular():
			if f.Mode()&0111 != 0 {
				fg = termbox.AttrBold | termbox.ColorGreen
			} else {
				fg = termbox.ColorDefault
			}
		case f.Mode().IsDir():
			fg = termbox.AttrBold | termbox.ColorBlue
		case f.Mode()&os.ModeSymlink != 0:
			fg = termbox.ColorCyan
		case f.Mode()&os.ModeNamedPipe != 0:
			fg = termbox.ColorRed
		case f.Mode()&os.ModeSocket != 0:
			fg = termbox.ColorYellow
		case f.Mode()&os.ModeDevice != 0:
			fg = termbox.ColorWhite
		}

		path := path.Join(dir.path, f.Name())

		if marks[path] {
			win.print(0, i, fg, termbox.ColorMagenta, " ")
		}

		if i == dir.pos {
			fg = fg | termbox.AttrReverse
		}

		var s []byte

		s = append(s, ' ')

		s = append(s, f.Name()...)

		if len(s) > win.w-2 {
			s = s[:win.w-2]
		} else {
			s = append(s, make([]byte, win.w-2-len(s))...)
		}

		switch gOpts.showinfo {
		case "none":
			break
		case "size":
			if win.w > 8 {
				h := humanize(f.Size())
				s = append(s[:win.w-3-len(h)])
				s = append(s, ' ')
				s = append(s, h...)
			}
		case "time":
			if win.w > 24 {
				t := f.ModTime().Format("Jan _2 15:04")
				s = append(s[:win.w-3-len(t)])
				s = append(s, ' ')
				s = append(s, t...)
			}
		default:
			log.Printf("unknown showinfo type: %s", gOpts.showinfo)
		}

		// TODO: add a trailing '~' to the name if cut

		win.print(1, i, fg, bg, string(s))
	}
}

func (win *Win) printr(reg *os.File) error {
	fg, bg := termbox.ColorDefault, termbox.ColorDefault

	buf := bufio.NewScanner(reg)

	for i := 0; i < win.h && buf.Scan(); i++ {
		for _, r := range buf.Text() {
			if unicode.IsSpace(r) {
				continue
			}
			if !unicode.IsPrint(r) {
				fg = termbox.AttrBold
				win.print(0, 0, fg, bg, "binary")
				return nil
			}
		}
	}

	if buf.Err() != nil {
		return fmt.Errorf("printing regular file: %s", buf.Err())
	}

	reg.Seek(0, 0)

	buf = bufio.NewScanner(reg)

	for i := 0; i < win.h && buf.Scan(); i++ {
		win.print(2, i, fg, bg, buf.Text())
	}

	if buf.Err() != nil {
		return fmt.Errorf("printing regular file: %s", buf.Err())
	}

	return nil
}

type UI struct {
	wins    []*Win
	pwdwin  *Win
	msgwin  *Win
	menuwin *Win
	message string
}

func getWidths(wtot int) []int {
	rsum := 0
	for _, rat := range gOpts.ratios {
		rsum += rat
	}

	wlen := len(gOpts.ratios)
	widths := make([]int, wlen)

	wsum := 0
	for i := 0; i < wlen-1; i++ {
		widths[i] = gOpts.ratios[i] * (wtot / rsum)
		wsum += widths[i]
	}
	widths[wlen-1] = wtot - wsum

	return widths
}

func newUI() *UI {
	wtot, htot := termbox.Size()

	var wins []*Win

	widths := getWidths(wtot)

	wacc := 0
	wlen := len(widths)
	for i := 0; i < wlen; i++ {
		wins = append(wins, newWin(widths[i], htot-2, wacc, 1))
		wacc += widths[i]
	}

	return &UI{
		wins:    wins,
		pwdwin:  newWin(wtot, 1, 0, 0),
		msgwin:  newWin(wtot, 1, 0, htot-1),
		menuwin: newWin(wtot, 1, 0, htot-2),
	}
}

func (ui *UI) renew() {
	termbox.Flush()

	wtot, htot := termbox.Size()

	widths := getWidths(wtot)

	wacc := 0
	wlen := len(widths)
	for i := 0; i < wlen; i++ {
		ui.wins[i].renew(widths[i], htot-2, wacc, 1)
		wacc += widths[i]
	}

	ui.msgwin.renew(wtot, 1, 0, htot-1)
}

func (ui *UI) echoFileInfo(nav *Nav) {
	dir := nav.currDir()

	if len(dir.fi) == 0 {
		return
	}

	curr := nav.currFile()

	ui.message = fmt.Sprintf("%v %v %v", curr.Mode(), humanize(curr.Size()), curr.ModTime().Format(time.ANSIC))
}

func (ui *UI) clearMsg() {
	fg, bg := termbox.ColorDefault, termbox.ColorDefault
	win := ui.msgwin
	win.printl(0, 0, fg, bg, "")
	termbox.SetCursor(win.x, win.y)
	termbox.Flush()
}

func (ui *UI) draw(nav *Nav) {
	fg, bg := termbox.ColorDefault, termbox.ColorDefault

	termbox.Clear(fg, bg)
	defer termbox.Flush()

	dir := nav.currDir()

	path := strings.Replace(dir.path, envHome, "~", -1)

	ui.pwdwin.printf(0, 0, termbox.AttrBold|termbox.ColorGreen, bg, "%s@%s", envUser, envHost)
	ui.pwdwin.printf(len(envUser)+len(envHost)+1, 0, fg, bg, ":")
	ui.pwdwin.printf(len(envUser)+len(envHost)+2, 0, termbox.AttrBold|termbox.ColorBlue, bg, "%s", path)

	length := min(len(ui.wins), len(nav.dirs))
	woff := len(ui.wins) - length

	if gOpts.preview {
		length = min(len(ui.wins)-1, len(nav.dirs))
		woff = len(ui.wins) - 1 - length
	}

	doff := len(nav.dirs) - length
	for i := 0; i < length; i++ {
		ui.wins[woff+i].printd(nav.dirs[doff+i], nav.marks)
	}

	defer ui.msgwin.print(0, 0, fg, bg, ui.message)

	if gOpts.preview {
		if len(dir.fi) == 0 {
			return
		}

		preview := ui.wins[len(ui.wins)-1]
		path := nav.currPath()

		f, err := os.Stat(path)
		if err != nil {
			msg := fmt.Sprintf("getting file information: %s", err)
			ui.message = msg
			log.Print(msg)
			return
		}

		if f.IsDir() {
			dir := newDir(path)
			dir.load(nav.inds[path], nav.poss[path], nav.height, nav.names[path])
			preview.printd(dir, nav.marks)
		} else if f.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				msg := fmt.Sprintf("opening file: %s", err)
				ui.message = msg
				log.Print(msg)
			}

			if err := preview.printr(file); err != nil {
				ui.message = err.Error()
				log.Print(err)
			}
		}
	}
}

func findBinds(keys map[string]Expr, prefix string) (binds map[string]Expr, ok bool) {
	binds = make(map[string]Expr)
	for key, expr := range keys {
		if strings.HasPrefix(key, prefix) {
			binds[key] = expr
			if key == prefix {
				ok = true
			}
		}
	}
	return
}

func (ui *UI) getExpr() Expr {
	r := &CallExpr{"redraw", nil}

	var acc []rune

	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			if ev.Ch != 0 {
				acc = append(acc, ev.Ch)
			} else {
				// TODO: rest of the keys
				switch ev.Key {
				case termbox.KeySpace:
					acc = append(acc, '<', 's', 'p', 'a', 'c', 'e', '>')
				case termbox.KeyEnter:
					acc = append(acc, '<', 'c', 'r', '>')
				case termbox.KeyBackspace:
					acc = append(acc, '<', 'b', 's', '>')
				case termbox.KeyBackspace2:
					acc = append(acc, '<', 'b', 's', '2', '>')
				case termbox.KeyTab:
					acc = append(acc, '<', 't', 'a', 'b', '>')
				case termbox.KeyArrowUp:
					acc = append(acc, '<', 'u', 'p', '>')
				case termbox.KeyArrowDown:
					acc = append(acc, '<', 'd', 'o', 'w', 'n', '>')
				case termbox.KeyArrowLeft:
					acc = append(acc, '<', 'l', 'e', 'f', 't', '>')
				case termbox.KeyArrowRight:
					acc = append(acc, '<', 'r', 'i', 'g', 'h', 't', '>')
				case termbox.KeyCtrlL:
					acc = append(acc, '<', 'c', '-', 'l', '>')
				case termbox.KeyEsc:
					acc = nil
					return r
				default:
					ui.message = fmt.Sprintf("unhandled key")
					acc = nil
					return r
				}
			}

			binds, ok := findBinds(gOpts.keys, string(acc))

			switch len(binds) {
			case 0:
				ui.message = fmt.Sprintf("unknown mapping: %s", string(acc))
				acc = nil
				return r
			case 1:
				if ok {
					return gOpts.keys[string(acc)]
				}
				ui.listBinds(binds)
			default:
				if ok {
					// TODO: use a delay
					return gOpts.keys[string(acc)]
				}
				ui.listBinds(binds)
			}
		case termbox.EventResize:
			return r
		default:
			// TODO: handle other events
		}
	}
}

func (ui *UI) prompt(pref string) string {
	fg, bg := termbox.ColorDefault, termbox.ColorDefault

	win := ui.msgwin

	win.printl(0, 0, fg, bg, pref)
	termbox.SetCursor(win.x+len(pref), win.y)
	defer termbox.HideCursor()
	termbox.Flush()

	var acc []rune

	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			if ev.Ch != 0 {
				acc = append(acc, ev.Ch)
			} else {
				// TODO: rest of the keys
				switch ev.Key {
				case termbox.KeySpace:
					acc = append(acc, ' ')
				case termbox.KeyBackspace2:
					if len(acc) > 0 {
						acc = acc[:len(acc)-1]
					}
				case termbox.KeyEnter:
					win.printl(0, 0, fg, bg, "")
					termbox.SetCursor(win.x, win.y)
					termbox.Flush()
					return string(acc)
				case termbox.KeyTab:
					if pref == ":" {
						acc = compCmd(acc)
					} else {
						acc = compShell(acc)
					}
				case termbox.KeyEsc:
					return ""
				}
			}

			win.printl(0, 0, fg, bg, pref)
			win.print(len(pref), 0, fg, bg, string(acc))
			termbox.SetCursor(win.x+len(pref)+len(acc), win.y)
			termbox.Flush()
		default:
			// TODO: handle other events
		}
	}
}

func (ui *UI) pause() {
	termbox.Close()
}

func (ui *UI) resume() {
	if err := termbox.Init(); err != nil {
		log.Fatalf("initializing termbox: %s", err)
	}
}

func (ui *UI) sync() {
	if err := termbox.Sync(); err != nil {
		log.Printf("syncing termbox: %s", err)
	}
	termbox.SetCursor(0, 0)
	termbox.HideCursor()
}

func (ui *UI) listBinds(binds map[string]Expr) {
	t := new(tabwriter.Writer)
	b := new(bytes.Buffer)

	t.Init(b, 0, 8, 0, '\t', 0)
	fmt.Fprintln(t, "keys\tcommand")
	for key, expr := range binds {
		fmt.Fprintf(t, "%s\t%v\n", key, expr)
	}
	t.Flush()

	lines := strings.Split(b.String(), "\n")

	lines = lines[:len(lines)-1]

	ui.menuwin.h = len(lines) - 1
	ui.menuwin.y = ui.wins[0].h - ui.menuwin.h

	ui.menuwin.printl(0, 0, termbox.AttrBold, termbox.AttrBold, lines[0])
	for i, line := range lines[1:] {
		ui.menuwin.printl(0, i+1, termbox.ColorDefault, termbox.ColorDefault, line)
	}

	termbox.Flush()
}
