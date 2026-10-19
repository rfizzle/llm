package openai

import (
	"strings"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/attachment"
	"github.com/rfizzle/llm/internal/llmtest"
)

func TestToOpenAIMessages_MixedContentClearsTheStringField(t *testing.T) {
	out := ChatMessages([]llm.Message{{
		Role:        llm.RoleUser,
		Content:     "look",
		Attachments: []attachment.Attachment{llmtest.PNG},
	}})
	msg := out[0]
	if msg.Content != "" {
		t.Fatal("Content and MultiContent are mutually exclusive in the SDK")
	}
	if len(msg.MultiContent) != 3 {
		t.Fatalf("got %d parts, want label + image + sentence", len(msg.MultiContent))
	}
	if msg.MultiContent[1].ImageURL == nil || msg.MultiContent[1].ImageURL.URL != llmtest.PNG.DataURL() {
		t.Fatal("the image part does not carry the data URL")
	}
	if msg.MultiContent[2].Text != "look" {
		t.Fatal("the sentence should come last")
	}
	// Messages without attachments stay on the plain string form.
	plain := ChatMessages([]llm.Message{{Role: llm.RoleUser, Content: "hi"}})
	if plain[0].Content != "hi" || plain[0].MultiContent != nil {
		t.Fatalf("plain message = %#v", plain[0])
	}
}

func TestAudioDegradesOnTheDialectsWithNoPartForIt(t *testing.T) {
	// Chat completions: the endpoint takes a recording and the client this
	// dialect goes through has no field for one, so the note is what rides.
	parts := ChatMessages([]llm.Message{{
		Role:        llm.RoleUser,
		Content:     "listen",
		Attachments: []attachment.Attachment{llmtest.MP3},
	}})[0].MultiContent
	if len(parts) != 3 || parts[1].ImageURL != nil {
		t.Fatalf("got %#v", parts)
	}
	if !strings.Contains(parts[1].Text, "could not be sent") || !strings.Contains(parts[1].Text, "memo.mp3") {
		t.Fatalf("fallback part = %q", parts[1].Text)
	}
}

func TestToOpenAIMessages_LabelLeadsEachAttachment(t *testing.T) {
	for _, tc := range llmtest.LabelledCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			parts := ChatMessages([]llm.Message{{
				Role: llm.RoleUser, Content: tc.Att.Handle + " shows it", Attachments: []attachment.Attachment{tc.Att},
			}})[0].MultiContent
			if len(parts) != 3 {
				t.Fatalf("got %d parts, want label, bytes, sentence", len(parts))
			}
			if parts[0].Text != tc.Label || parts[0].ImageURL != nil {
				t.Fatalf("first part = %#v, want the label %q", parts[0], tc.Label)
			}
			if tc.Att.Kind == attachment.Image {
				if parts[1].ImageURL == nil {
					t.Fatal("the picture should follow its label")
				}
			} else if parts[1].Text != tc.Att.AsText() {
				t.Fatalf("second part = %q, want the attachment's text form", parts[1].Text)
			}
			if parts[2].Text != tc.Att.Handle+" shows it" {
				t.Fatal("the sentence should come last")
			}
		})
	}
}
