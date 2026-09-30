package alert

import (
	"fmt"
	"html"
	"strings"
)

// EmailMIME builds a multipart/alternative message body (without SMTP headers).
func EmailMIME(p Payload) (plain string, htmlBody string) {
	plain = EmailBody(p)
	htmlBody = emailHTML(p)
	return plain, htmlBody
}

func emailHTML(p Payload) string {
	title := html.EscapeString(p.Text)
	ups := html.EscapeString(p.UPSName)
	event := html.EscapeString(p.Event)
	state := html.EscapeString(p.State)
	prev := html.EscapeString(p.PreviousState)
	when := html.EscapeString(p.Timestamp)
	accent := "#b8612d"
	switch p.Event {
	case string(KindOnBattery):
		accent = "#c50f1f"
	case string(KindOnMains):
		accent = "#107c10"
	case string(KindLowBattery):
		accent = "#ff8c00"
	}
	rows := []string{
		row("UPS", ups),
		row("Event", event),
		row("Status", state),
		row("Previous", prev),
		row("Battery", factValueHTML(p.ChargePercent, "%")),
		row("Runtime", factValueHTML(p.MinutesRemaining, " min")),
		row("Load", factValueHTML(p.LoadPercent, "%")),
		row("Time", when),
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="UTF-8"></head>
<body style="font-family:Segoe UI,Arial,sans-serif;background:#f3ecdf;color:#1c1612;padding:24px">
<div style="max-width:520px;margin:0 auto;background:#efe4d2;border:1px solid #d4c4ad;border-radius:8px;overflow:hidden">
<div style="background:%s;color:#fff;padding:16px 20px;font-size:18px;font-weight:600">%s</div>
<table style="width:100%%;border-collapse:collapse;padding:12px">%s</table>
</div>
<p style="max-width:520px;margin:16px auto 0;font-size:12px;color:#6e6256">Sent by NUT Allergy</p>
</body></html>`, accent, title, strings.Join(rows, ""))
}

func row(label, value string) string {
	return fmt.Sprintf(
		`<tr><td style="padding:10px 20px;color:#6e6256;font-size:13px;width:120px;vertical-align:top">%s</td><td style="padding:10px 20px;font-size:14px">%s</td></tr>`,
		html.EscapeString(label),
		value,
	)
}

func factValueHTML(p *int, suffix string) string {
	if p == nil {
		return "—"
	}
	return html.EscapeString(fmt.Sprintf("%d%s", *p, suffix))
}
