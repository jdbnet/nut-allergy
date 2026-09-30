package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0a15a")).Bold(true)
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#b3a394"))
	markStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#b8612d"))
)

type catalogMsg struct {
	resp catalogResponse
}
type errMsg struct{ err error }
type savedMsg struct{}

type tuiModel struct {
	cfg      agentConfig
	path     string
	clientOK bool
	ups      catalogResponse
	cursor   int
	selected map[string]bool
	timeout  string
	stage    string
	errText  string
}

func runSetup(configPath string) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w (enroll before setup)", err)
	}
	m := tuiModel{
		cfg:      cfg,
		path:     configPath,
		selected: map[string]bool{},
		stage:    "load",
	}
	for _, id := range cfg.UPSIDs {
		m.selected[id] = true
	}
	if cfg.TimeoutOverride != nil {
		m.timeout = fmt.Sprintf("%d", *cfg.TimeoutOverride)
	}
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func (m tuiModel) Init() tea.Cmd { return m.fetch }

func (m tuiModel) fetch() tea.Msg {
	client, err := httpClient(m.cfg)
	if err != nil {
		return errMsg{err}
	}
	var resp catalogResponse
	if err := getJSON(client, m.cfg.Server+"/api/agent/catalog", &resp); err != nil {
		return errMsg{err}
	}
	return catalogMsg{resp}
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case catalogMsg:
		m.ups = msg.resp
		m.stage = "select"
		if len(m.selected) == 0 {
			for _, id := range msg.resp.UPSIDs {
				m.selected[id] = true
			}
		}
		return m, nil
	case errMsg:
		m.stage = "error"
		m.errText = msg.err.Error()
		return m, nil
	case savedMsg:
		m.stage = "done"
		return m, tea.Quit
	case tea.KeyMsg:
		if m.stage == "error" && msg.String() == "q" {
			return m, tea.Quit
		}
		switch m.stage {
		case "select":
			return m.updateSelect(msg)
		case "timeout":
			return m.updateTimeout(msg)
		}
	}
	return m, nil
}

func (m tuiModel) updateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.ups.UPS)-1 {
			m.cursor++
		}
	case " ":
		if len(m.ups.UPS) == 0 {
			return m, nil
		}
		id := m.ups.UPS[m.cursor].ID
		m.selected[id] = !m.selected[id]
	case "enter":
		m.stage = "timeout"
	}
	return m, nil
}

func (m tuiModel) updateTimeout(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.stage = "select"
	case "enter":
		return m, m.putConfig
	case "backspace":
		if len(m.timeout) > 0 {
			m.timeout = m.timeout[:len(m.timeout)-1]
		}
	default:
		if len(msg.String()) == 1 && msg.String()[0] >= '0' && msg.String()[0] <= '9' && len(m.timeout) < 6 {
			m.timeout += msg.String()
		}
	}
	return m, nil
}

func (m tuiModel) putConfig() tea.Msg {
	client, err := httpClient(m.cfg)
	if err != nil {
		return errMsg{err}
	}
	ids := make([]string, 0)
	for _, u := range m.ups.UPS {
		if m.selected[u.ID] {
			ids = append(ids, u.ID)
		}
	}
	body := map[string]any{"ups_ids": ids}
	if strings.TrimSpace(m.timeout) == "" {
		body["timeout_override_seconds"] = nil
	} else {
		var n int
		if _, err := fmt.Sscanf(m.timeout, "%d", &n); err != nil || n < 1 {
			return errMsg{fmt.Errorf("timeout must be seconds, or blank for the server default")}
		}
		body["timeout_override_seconds"] = n
	}
	req, err := jsonRequest("PUT", m.cfg.Server+"/api/agent/config", body)
	if err != nil {
		return errMsg{err}
	}
	res, err := client.Do(req)
	if err != nil {
		return errMsg{err}
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return errMsg{fmt.Errorf("save config: %s", res.Status)}
	}
	return savedMsg{}
}

func (m tuiModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("NUT Allergy agent") + "\n")
	switch m.stage {
	case "load":
		b.WriteString("Loading UPSs from the server…\n")
	case "error":
		b.WriteString("Error: " + m.errText + "\n")
		b.WriteString(dimStyle.Render("Press q to quit") + "\n")
	case "select":
		b.WriteString("Select every UPS this machine is plugged into.\n")
		b.WriteString(dimStyle.Render("space toggles, enter continues, q quits") + "\n\n")
		if len(m.ups.UPS) == 0 {
			b.WriteString("No UPSs are configured on the server yet.\n")
		}
		for i, u := range m.ups.UPS {
			mark := " "
			if m.selected[u.ID] {
				mark = markStyle.Render("●")
			} else {
				mark = "○"
			}
			cursor := " "
			if i == m.cursor {
				cursor = ">"
			}
			line := fmt.Sprintf("%s %s %s", cursor, mark, u.Name)
			if u.Description != "" {
				line += dimStyle.Render("  " + u.Description)
			}
			b.WriteString(line + "\n")
		}
	case "timeout":
		b.WriteString("Optional shutdown wait in seconds.\n")
		b.WriteString(dimStyle.Render("Blank uses the server default. esc goes back.") + "\n\n")
		shown := m.timeout
		if shown == "" {
			shown = dimStyle.Render("(server default)")
		}
		b.WriteString("Timeout: " + shown + "\n")
	case "done":
		b.WriteString("Saved. The service picks this up on its next poll.\n")
	}
	return b.String()
}
