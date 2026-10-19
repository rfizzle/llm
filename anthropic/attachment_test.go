package anthropic

import (
	"strings"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/attachment"
	"github.com/rfizzle/llm/internal/llmtest"
)

func TestToAnthropicMessages_ImageLeadsTheMessage(t *testing.T) {
	_, out := toAnthropicMessages([]llm.Message{{
		Role:        llm.RoleUser,
		Content:     "what is wrong here?",
		Attachments: []attachment.Attachment{llmtest.PNG, llmtest.Notes},
	}})
	if len(out) != 1 {
		t.Fatalf("got %d messages, want 1", len(out))
	}
	blocks := out[0].Content
	if len(blocks) != 5 {
		t.Fatalf("got %d blocks, want a label and the bytes per attachment, then the sentence", len(blocks))
	}
	if blocks[1].OfImage == nil {
		t.Fatal("the image should lead the message")
	}
	if blocks[1].OfImage.Source.OfBase64.Data != llmtest.PNG.Base64() {
		t.Fatal("image block does not carry the bytes")
	}
	if blocks[4].OfText == nil || blocks[4].OfText.Text != "what is wrong here?" {
		t.Fatal("the sentence should come last")
	}
}

func TestToAnthropicMessages_EmptySentenceStillSendsOneBlock(t *testing.T) {
	// Attaching and pressing enter with nothing typed must not produce a
	// message with no content at all, which the API rejects.
	_, out := toAnthropicMessages([]llm.Message{{Role: llm.RoleUser, Attachments: []attachment.Attachment{llmtest.PNG}}})
	if len(out) != 1 || len(out[0].Content) != 2 || out[0].Content[1].OfImage == nil {
		t.Fatalf("got %#v", out)
	}
	// And a message with no attachments keeps its single text block.
	_, plain := toAnthropicMessages([]llm.Message{{Role: llm.RoleUser, Content: "hi"}})
	if len(plain[0].Content) != 1 || plain[0].Content[0].OfText == nil {
		t.Fatal("a plain message should still be one text block")
	}
}

func TestAudioDegradesOnTheDialectsWithNoPartForIt(t *testing.T) {
	// The Messages API takes no recording at all.
	_, out := toAnthropicMessages([]llm.Message{{Role: llm.RoleUser, Attachments: []attachment.Attachment{llmtest.MP3}}})
	block := out[0].Content[1]
	if block.OfText == nil || !strings.Contains(block.OfText.Text, "could not be sent") {
		t.Fatalf("anthropic block = %#v", block)
	}
}

func TestToAnthropicMessages_LabelLeadsEachAttachment(t *testing.T) {
	for _, tc := range llmtest.LabelledCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			_, out := toAnthropicMessages([]llm.Message{{
				Role: llm.RoleUser, Content: tc.Att.Handle + " shows it", Attachments: []attachment.Attachment{tc.Att},
			}})
			blocks := out[0].Content
			if len(blocks) != 3 {
				t.Fatalf("got %d blocks, want label, bytes, sentence", len(blocks))
			}
			if blocks[0].OfText == nil || blocks[0].OfText.Text != tc.Label {
				t.Fatalf("first block = %#v, want the label %q", blocks[0].OfText, tc.Label)
			}
			switch tc.Att.Kind {
			case attachment.Image:
				if blocks[1].OfImage == nil {
					t.Fatal("the picture should follow its label")
				}
			case attachment.Document:
				if blocks[1].OfDocument == nil {
					t.Fatal("the document should follow its label")
				}
			default:
				if blocks[1].OfText == nil || !strings.Contains(blocks[1].OfText.Text, `<attachment name="`+tc.Att.Name+`"`) {
					t.Fatalf("the text attachment should keep its wrapper: %#v", blocks[1].OfText)
				}
			}
			if blocks[2].OfText == nil || blocks[2].OfText.Text != tc.Att.Handle+" shows it" {
				t.Fatal("the sentence should come last")
			}
		})
	}
}
