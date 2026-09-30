package webhooks

import (
	"encoding/json"
	"testing"

)

func templates() []string {
	return []string{TemplateTeams, TemplateDiscord, TemplateSlack, TemplateGotify, TemplateNtfy, TemplateGeneric}
}

func events() []string {
	return []string{"test", "on_battery", "on_mains", "low_battery"}
}

func TestBuildEachTemplateAndEvent(t *testing.T) {
	for _, tmpl := range templates() {
		for _, ev := range events() {
			p := SampleMessage(ev)
			d, err := Build(tmpl, p)
			if err != nil {
				t.Fatalf("%s %s: %v", tmpl, ev, err)
			}
			if len(d.Body) == 0 {
				t.Fatalf("%s %s: empty body", tmpl, ev)
			}
			if d.Headers["Content-Type"] != "application/json" {
				t.Fatalf("%s: content-type %q", tmpl, d.Headers["Content-Type"])
			}
			switch tmpl {
			case TemplateTeams:
				assertTeams(t, d.Body, ev)
			case TemplateDiscord:
				assertDiscord(t, d.Body, ev)
			case TemplateSlack:
				assertSlack(t, d.Body)
			case TemplateGotify:
				assertGotify(t, d.Body, ev)
			case TemplateNtfy:
				assertNtfy(t, d.Body, ev)
			case TemplateGeneric:
				assertGeneric(t, d.Body, p)
			}
		}
	}
}

func assertTeams(t *testing.T, raw []byte, event string) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["type"] != "message" {
		t.Fatalf("type=%v", m["type"])
	}
	atts, ok := m["attachments"].([]any)
	if !ok || len(atts) != 1 {
		t.Fatalf("attachments=%v", m["attachments"])
	}
	att, _ := atts[0].(map[string]any)
	if att["contentType"] != "application/vnd.microsoft.card.adaptive" {
		t.Fatalf("contentType=%v", att["contentType"])
	}
	content, _ := att["content"].(map[string]any)
	if content["type"] != "AdaptiveCard" {
		t.Fatalf("card type=%v", content["type"])
	}
	body, _ := content["body"].([]any)
	if len(body) < 2 {
		t.Fatal("card body too short")
	}
	title, _ := body[0].(map[string]any)
	want := teamsTitleColor(event)
	if title["color"] != want {
		t.Fatalf("title color=%v want %s", title["color"], want)
	}
}

func assertDiscord(t *testing.T, raw []byte, event string) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	embeds, ok := m["embeds"].([]any)
	if !ok || len(embeds) != 1 {
		t.Fatalf("embeds=%v", m["embeds"])
	}
	emb, _ := embeds[0].(map[string]any)
	if int(emb["color"].(float64)) != discordColor(event) {
		t.Fatalf("color=%v", emb["color"])
	}
}

func assertSlack(t *testing.T, raw []byte) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["text"] == "" {
		t.Fatal("missing text fallback")
	}
	blocks, ok := m["blocks"].([]any)
	if !ok || len(blocks) < 2 {
		t.Fatalf("blocks=%v", m["blocks"])
	}
}

func assertGotify(t *testing.T, raw []byte, event string) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["title"] == "" || m["message"] == "" {
		t.Fatal("missing title/message")
	}
	if int(m["priority"].(float64)) != gotifyPriority(event) {
		t.Fatalf("priority=%v", m["priority"])
	}
	extras, _ := m["extras"].(map[string]any)
	display, _ := extras["client::display"].(map[string]any)
	if display["contentType"] != "text/markdown" {
		t.Fatalf("extras=%v", extras)
	}
}

func assertNtfy(t *testing.T, raw []byte, event string) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["title"] == "" || m["message"] == "" {
		t.Fatal("missing title/message")
	}
	if int(m["priority"].(float64)) != ntfyPriority(event) {
		t.Fatalf("priority=%v", m["priority"])
	}
	tags, _ := m["tags"].([]any)
	if len(tags) != len(ntfyTags(event)) {
		t.Fatalf("tags=%v", tags)
	}
}

func assertGeneric(t *testing.T, raw []byte, p Message) {
	var got Message
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Event != p.Event || got.UPSName != p.UPSName {
		t.Fatalf("payload mismatch %#v", got)
	}
}
