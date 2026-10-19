package responses

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/attachment"
	"github.com/rfizzle/llm/internal/llmtest"
)

func TestToResponseItems_ImageAndDocumentParts(t *testing.T) {
	pdf := attachment.Attachment{Kind: attachment.Document, Name: "spec.pdf", MediaType: "application/pdf", Data: []byte("%PDF")}
	items, _ := toResponseItems([]llm.Message{{
		Role:        llm.RoleUser,
		Content:     "read this",
		Attachments: []attachment.Attachment{llmtest.PNG, pdf},
	}}, false)
	if len(items) != 1 || len(items[0].Content) != 5 {
		t.Fatalf("got %#v", items)
	}
	if items[0].Content[1].Type != "input_image" || items[0].Content[1].ImageURL != llmtest.PNG.DataURL() {
		t.Fatalf("image part = %#v", items[0].Content[1])
	}
	if items[0].Content[3].Type != "input_file" || items[0].Content[3].Filename != "spec.pdf" {
		t.Fatalf("file part = %#v", items[0].Content[3])
	}
	if items[0].Content[4].Type != "input_text" || items[0].Content[4].Text != "read this" {
		t.Fatalf("text part = %#v", items[0].Content[4])
	}
	// The added fields must stay out of an ordinary text part's JSON.
	b, err := json.Marshal(items[0].Content[4])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "image_url") || strings.Contains(string(b), "file_data") {
		t.Fatalf("text part carries empty attachment fields: %s", b)
	}
}

func TestToResponseItems_AudioIsBareBase64AndAFormatToken(t *testing.T) {
	items, _ := toResponseItems([]llm.Message{{
		Role:        llm.RoleUser,
		Content:     "transcribe this",
		Attachments: []attachment.Attachment{llmtest.MP3, llmtest.WAV},
	}}, false)
	if len(items) != 1 || len(items[0].Content) != 5 {
		t.Fatalf("got %#v", items)
	}
	mp3 := items[0].Content[1]
	if mp3.Type != "input_audio" || mp3.InputAudio == nil {
		t.Fatalf("mp3 part = %#v", mp3)
	}
	// Bare base64, not the data URL an image rides on.
	if mp3.InputAudio.Data != llmtest.MP3.Base64() || mp3.InputAudio.Format != "mp3" {
		t.Fatalf("mp3 audio = %#v", mp3.InputAudio)
	}
	if strings.HasPrefix(mp3.InputAudio.Data, "data:") {
		t.Fatal("the audio part takes base64 without a data URL prefix")
	}
	if wav := items[0].Content[3]; wav.InputAudio == nil || wav.InputAudio.Format != "wav" {
		t.Fatalf("wav part = %#v", wav)
	}
	if items[0].Content[4].Type != "input_text" {
		t.Fatalf("text part = %#v", items[0].Content[4])
	}
	// The nested object must stay out of every other part's JSON.
	b, err := json.Marshal(items[0].Content[4])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "input_audio") {
		t.Fatalf("text part carries an empty audio object: %s", b)
	}
}

func TestToResponseItems_AFormatTheAudioPartOmitsGoesAsText(t *testing.T) {
	items, _ := toResponseItems([]llm.Message{{Role: llm.RoleUser, Attachments: []attachment.Attachment{llmtest.OGG}}}, false)
	part := items[0].Content[1]
	if part.Type != "input_text" || part.InputAudio != nil {
		t.Fatalf("ogg part = %#v", part)
	}
	if !strings.Contains(part.Text, "could not be sent") || !strings.Contains(part.Text, "call.ogg") {
		t.Fatalf("fallback part = %q", part.Text)
	}
}

func TestToResponseItems_LabelLeadsEachAttachment(t *testing.T) {
	bytesType := map[attachment.Kind]string{
		attachment.Image: "input_image", attachment.Document: "input_file", attachment.Text: "input_text",
	}
	for _, tc := range llmtest.LabelledCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			items, _ := toResponseItems([]llm.Message{{
				Role: llm.RoleUser, Content: tc.Att.Handle + " shows it", Attachments: []attachment.Attachment{tc.Att},
			}}, false)
			content := items[0].Content
			if len(content) != 3 {
				t.Fatalf("got %d parts, want label, bytes, sentence", len(content))
			}
			if content[0].Type != "input_text" || content[0].Text != tc.Label {
				t.Fatalf("first part = %#v, want the label %q", content[0], tc.Label)
			}
			if content[1].Type != bytesType[tc.Att.Kind] {
				t.Fatalf("second part = %q, want %q", content[1].Type, bytesType[tc.Att.Kind])
			}
			if content[2].Text != tc.Att.Handle+" shows it" {
				t.Fatal("the sentence should come last")
			}
		})
	}
}
