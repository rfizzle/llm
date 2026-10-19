package gemini

import (
	"strings"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/attachment"
	"github.com/rfizzle/llm/internal/llmtest"
)

func TestToGeminiContents_InlineBlobLeads(t *testing.T) {
	contents, _ := toGeminiContents([]llm.Message{{
		Role:        llm.RoleUser,
		Content:     "look",
		Attachments: []attachment.Attachment{llmtest.PNG},
	}})
	if len(contents) != 1 || len(contents[0].Parts) != 3 {
		t.Fatalf("got %#v", contents)
	}
	blob := contents[0].Parts[1].InlineData
	if blob == nil || blob.MIMEType != "image/png" || string(blob.Data) != string(llmtest.PNG.Data) {
		t.Fatalf("inline blob = %#v", blob)
	}
	if contents[0].Parts[2].Text != "look" {
		t.Fatal("the sentence should come last")
	}
}

func TestToGeminiContents_AudioRidesAsAnInlineBlob(t *testing.T) {
	contents, _ := toGeminiContents([]llm.Message{{
		Role:        llm.RoleUser,
		Content:     "what is said here?",
		Attachments: []attachment.Attachment{llmtest.MP3, llmtest.OGG},
	}})
	if len(contents) != 1 || len(contents[0].Parts) != 5 {
		t.Fatalf("got %#v", contents)
	}
	// Gemini's own list spells MP3 differently from every other surface.
	mp3 := contents[0].Parts[1].InlineData
	if mp3 == nil || mp3.MIMEType != "audio/mp3" || string(mp3.Data) != string(llmtest.MP3.Data) {
		t.Fatalf("mp3 blob = %#v", mp3)
	}
	ogg := contents[0].Parts[3].InlineData
	if ogg == nil || ogg.MIMEType != "audio/ogg" {
		t.Fatalf("ogg blob = %#v", ogg)
	}
	if contents[0].Parts[4].Text != "what is said here?" {
		t.Fatal("the sentence should come last")
	}
}

func TestToGeminiContents_AFormatItsListOmitsGoesAsText(t *testing.T) {
	flac := attachment.Attachment{Kind: attachment.Audio, Name: "take.flac", MediaType: "audio/flac", Data: []byte("fLaC")}
	contents, _ := toGeminiContents([]llm.Message{{Role: llm.RoleUser, Attachments: []attachment.Attachment{flac}}})
	part := contents[0].Parts[1]
	if part.InlineData != nil {
		t.Fatalf("a format the list omits should not go inline: %#v", part.InlineData)
	}
	if !strings.Contains(part.Text, "could not be sent") || !strings.Contains(part.Text, "take.flac") {
		t.Fatalf("fallback part = %q", part.Text)
	}
}

func TestToGeminiContents_LabelLeadsEachAttachment(t *testing.T) {
	for _, tc := range llmtest.LabelledCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			contents, _ := toGeminiContents([]llm.Message{{
				Role: llm.RoleUser, Content: tc.Att.Handle + " shows it", Attachments: []attachment.Attachment{tc.Att},
			}})
			parts := contents[0].Parts
			if len(parts) != 3 {
				t.Fatalf("got %d parts, want label, bytes, sentence", len(parts))
			}
			if parts[0].Text != tc.Label || parts[0].InlineData != nil {
				t.Fatalf("first part = %#v, want the label %q", parts[0], tc.Label)
			}
			if tc.Att.Kind == attachment.Text {
				if !strings.Contains(parts[1].Text, `<attachment name="`+tc.Att.Name+`"`) {
					t.Fatalf("the text attachment should keep its wrapper: %q", parts[1].Text)
				}
			} else if parts[1].InlineData == nil {
				t.Fatal("the bytes should follow their label")
			}
			if parts[2].Text != tc.Att.Handle+" shows it" {
				t.Fatal("the sentence should come last")
			}
		})
	}
}
