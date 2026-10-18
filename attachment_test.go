package llm_test

import (
	"strings"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/internal/llmtest"
)

func TestAttachment_DataURL(t *testing.T) {
	if got, want := llmtest.PNG.DataURL(), "data:image/png;base64,iVBORw=="; got != want {
		t.Fatalf("DataURL = %q, want %q", got, want)
	}
}

func TestAttachment_AsTextWrapsTheContents(t *testing.T) {
	got := llmtest.Notes.AsText()
	if !strings.Contains(got, `name="notes.md"`) || !strings.Contains(got, "# heading") {
		t.Fatalf("AsText = %q", got)
	}
	// Bytes no provider can carry say so rather than arriving as mojibake.
	note := llmtest.PNG.AsText()
	if !strings.Contains(note, "could not be sent") || !strings.Contains(note, "shot.png") {
		t.Fatalf("fallback note = %q", note)
	}
}

func TestAttachmentBytes(t *testing.T) {
	if got := llm.AttachmentBytes([]llm.Attachment{llmtest.PNG, llmtest.Notes}); got != len(llmtest.PNG.Data)+len(llmtest.Notes.Data) {
		t.Fatalf("AttachmentBytes = %d", got)
	}
}

func TestAudioMediaType_RenamesWhatTheSnifferAnswers(t *testing.T) {
	// The two the sniffing rules and the vendors' lists disagree about.
	if got, ok := llm.AudioMediaType("audio/wave"); !ok || got != "audio/wav" {
		t.Fatalf("audio/wave = %q, %v", got, ok)
	}
	if got, ok := llm.AudioMediaType("application/ogg"); !ok || got != "audio/ogg" {
		t.Fatalf("application/ogg = %q, %v", got, ok)
	}
	// And the two they agree about.
	for _, mt := range []string{"audio/mpeg", "audio/aiff"} {
		if got, ok := llm.AudioMediaType(mt); !ok || got != mt {
			t.Fatalf("%s = %q, %v", mt, got, ok)
		}
	}
	// A score is not a recording, and neither is a picture.
	for _, mt := range []string{"audio/midi", "image/png", "application/octet-stream"} {
		if got, ok := llm.AudioMediaType(mt); ok {
			t.Fatalf("%s carried as %q", mt, got)
		}
	}
}

func TestAttachment_LabelNamesTheHandleTheNameAndTheFigure(t *testing.T) {
	for _, tc := range llmtest.LabelledCases(t) {
		if got := tc.Att.Label(); got != tc.Label {
			t.Errorf("%s: Label = %q, want %q", tc.Name, got, tc.Label)
		}
	}
	// A picture whose header cannot be read is counted in bytes, and a row
	// saved before handles existed is named by its name alone.
	old := llm.Attachment{Kind: llm.AttachmentImage, Name: "shot.webp", MediaType: "image/webp", Data: []byte("RIFF")}
	if got, want := old.Label(), "shot.webp (4 B):"; got != want {
		t.Errorf("Label = %q, want %q", got, want)
	}
	one := llm.Attachment{Kind: llm.AttachmentText, Name: "paste-2.txt", Handle: "Paste#2", Data: []byte("x\n")}
	if got, want := one.Figure(), "1 line"; got != want {
		t.Errorf("Figure = %q, want %q", got, want)
	}
}
