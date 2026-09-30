package webhooks

// Template identifiers stored on each webhook row.
const (
	TemplateGeneric = "generic"
	TemplateTeams   = "teams"
	TemplateDiscord = "discord"
	TemplateSlack   = "slack"
	TemplateGotify  = "gotify"
	TemplateNtfy    = "ntfy"
)

// Info describes a webhook template for the UI.
type Info struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Help        string `json:"help,omitempty"`
}

// List returns supported webhook templates.
func List() []Info {
	return []Info{
		{
			ID:          TemplateTeams,
			Label:       "Microsoft Teams (Workflows)",
			Description: "Modern Teams Workflows incoming webhook with an Adaptive Card.",
			Help:        "In Teams: open the channel → Workflows → create a flow from \"Post to a channel when a webhook request is received\" → copy the HTTP POST URL.",
		},
		{ID: TemplateDiscord, Label: "Discord", Description: "Incoming webhook with a coloured embed."},
		{ID: TemplateSlack, Label: "Slack", Description: "Incoming webhook with Block Kit sections."},
		{ID: TemplateGotify, Label: "Gotify", Description: "POST to your Gotify message URL (include ?token=)."},
		{ID: TemplateNtfy, Label: "ntfy", Description: "POST JSON to your topic URL (token in URL if needed)."},
		{ID: TemplateGeneric, Label: "Generic JSON", Description: "Plain JSON payload for custom integrations."},
	}
}

// Normalize maps legacy or empty values to a known template id.
func Normalize(template string) string {
	switch template {
	case "", TemplateGeneric:
		return TemplateGeneric
	case TemplateTeams, TemplateDiscord, TemplateSlack, TemplateGotify, TemplateNtfy:
		return template
	default:
		return TemplateGeneric
	}
}
