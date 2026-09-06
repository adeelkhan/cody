package terminal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEncodeKeyRunesPassThrough(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hi")})
	if string(got) != "hi" {
		t.Fatalf("got %q, want %q", got, "hi")
	}
}

func TestEncodeKeySpace(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeySpace})
	if string(got) != " " {
		t.Fatalf("got %q, want %q", got, " ")
	}
}

func TestEncodeKeyEnterSendsCarriageReturn(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeyEnter})
	if string(got) != "\r" {
		t.Fatalf("got %q, want %q", got, "\r")
	}
}

func TestEncodeKeyTabSendsHorizontalTab(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeyTab})
	if string(got) != "\t" {
		t.Fatalf("got %q, want %q", got, "\t")
	}
}

func TestEncodeKeyEscSendsEscapeByte(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeyEsc})
	if len(got) != 1 || got[0] != 0x1b {
		t.Fatalf("got %v, want [0x1b]", got)
	}
}

func TestEncodeKeyBackspaceSendsDEL(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if len(got) != 1 || got[0] != 0x7f {
		t.Fatalf("got %v, want [0x7f]", got)
	}
}

func TestEncodeKeyCtrlCSendsETX(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if len(got) != 1 || got[0] != 0x03 {
		t.Fatalf("got %v, want [0x03]", got)
	}
}

func TestEncodeKeyCtrlZSendsSUB(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeyCtrlZ})
	if len(got) != 1 || got[0] != 0x1a {
		t.Fatalf("got %v, want [0x1a]", got)
	}
}

func TestEncodeKeyArrowsSendStandardXtermSequences(t *testing.T) {
	cases := map[tea.KeyType]string{
		tea.KeyUp:    "\x1b[A",
		tea.KeyDown:  "\x1b[B",
		tea.KeyRight: "\x1b[C",
		tea.KeyLeft:  "\x1b[D",
		tea.KeyHome:  "\x1b[H",
		tea.KeyEnd:   "\x1b[F",
	}
	for kt, want := range cases {
		got := encodeKey(tea.KeyMsg{Type: kt})
		if string(got) != want {
			t.Errorf("key %v: got %q, want %q", kt, got, want)
		}
	}
}

func TestEncodeKeyPageAndDeleteSendStandardSequences(t *testing.T) {
	cases := map[tea.KeyType]string{
		tea.KeyPgUp:   "\x1b[5~",
		tea.KeyPgDown: "\x1b[6~",
		tea.KeyDelete: "\x1b[3~",
		tea.KeyInsert: "\x1b[2~",
	}
	for kt, want := range cases {
		got := encodeKey(tea.KeyMsg{Type: kt})
		if string(got) != want {
			t.Errorf("key %v: got %q, want %q", kt, got, want)
		}
	}
}

func TestEncodeKeyReturnsNilForUnencodableKeys(t *testing.T) {
	if got := encodeKey(tea.KeyMsg{Type: tea.KeyShiftTab}); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}
