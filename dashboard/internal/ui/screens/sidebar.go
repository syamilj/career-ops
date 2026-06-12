package screens

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/santifer/career-ops/dashboard/internal/model"
	"github.com/santifer/career-ops/dashboard/internal/theme"
)

// SidebarModel implements the sidebar panel.
type SidebarModel struct {
	sections  []SidebarItem
	width     int
	height    int
	theme     theme.Theme
	collapsed bool
	metrics   model.PipelineMetrics
	sortMode  string
	focused   bool
}

type SidebarItem struct {
	Label    string
	Count    int
	Color    lipgloss.Color
	Action   string
	Selected bool
}

func NewSidebarModel(t theme.Theme, apps []model.CareerApplication, metrics model.PipelineMetrics, width, height int) SidebarModel {
	statusCounts := make(map[string]int)
	for _, app := range apps {
		statusCounts[normalizeAppStatus(app.Status)]++
	}

	return SidebarModel{
		sections: []SidebarItem{
			{Label: "All", Count: metrics.Total, Color: lipgloss.Color("15"), Action: "all"},
			{Label: "Evaluated", Count: statusCounts["evaluated"], Color: lipgloss.Color("15"), Action: "evaluated"},
			{Label: "Applied", Count: statusCounts["applied"], Color: lipgloss.Color("12"), Action: "applied"},
			{Label: "Interview", Count: statusCounts["interview"], Color: lipgloss.Color("10"), Action: "interview"},
			{Label: "Rejected", Count: statusCounts["rejected"], Color: lipgloss.Color("9"), Action: "rejected"},
			{Label: "Skip", Count: statusCounts["skip"], Color: lipgloss.Color("11"), Action: "skip"},
		},
		width:   width,
		height:  height,
		theme:   t,
		metrics: metrics,
		sortMode: sortScore,
	}
}

func normalizeAppStatus(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "**", "")
	return strings.TrimSpace(s)
}

func (m *SidebarModel) ToggleCollapse() { m.collapsed = !m.collapsed }
func (m SidebarModel) Width() int       { if m.collapsed { return 3 }; return m.width }
func (m *SidebarModel) Resize(w, h int) { m.width = w; m.height = h }
func (m SidebarModel) Init() tea.Cmd    { return nil }
func (m *SidebarModel) SetMetrics(met model.PipelineMetrics) { m.metrics = met }
func (m *SidebarModel) SetSortMode(mode string) { m.sortMode = mode }

var activeItem int

func (m SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	if m.collapsed {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if activeItem > 0 {
				activeItem--
			}
		case "down", "j":
			if activeItem < len(m.sections)-1 {
				activeItem++
			}
		case "enter", " ":
			if activeItem >= 0 && activeItem < len(m.sections) {
				m.sections[activeItem].Selected = !m.sections[activeItem].Selected
			}
		}
	case tea.MouseMsg:
		if msg.Type == tea.MouseLeft {
			// Calculate which item was clicked
			clickY := msg.Y - 5 // offset for header
			if clickY >= 0 && clickY < len(m.sections) {
				activeItem = clickY
				m.sections[activeItem].Selected = !m.sections[activeItem].Selected
			}
		}
	}
	return m, nil
}

func (m SidebarModel) GetSelectedFilters() []string {
	var filters []string
	for _, item := range m.sections {
		if item.Selected {
			filters = append(filters, item.Action)
		}
	}
	return filters
}

func (m SidebarModel) View() string {
	if m.collapsed {
		return m.renderCollapsed()
	}
	return m.renderExpanded()
}

func (m SidebarModel) renderCollapsed() string {
	style := lipgloss.NewStyle().
		Width(3).
		Height(m.height).
		Background(m.theme.Surface).
		Foreground(m.theme.Text).
		Align(lipgloss.Center)
	return lipgloss.JoinVertical(lipgloss.Left,
		style.Render("≡"),
		"",
		style.Render("▶"),
	)
}

func (m SidebarModel) renderExpanded() string {
	var lines []string

	// Header
	hdr := lipgloss.NewStyle().
		Width(m.width).
		Background(m.theme.Surface).
		Foreground(m.theme.Blue).
		Bold(true).
		Padding(0, 1)
	lines = append(lines, hdr.Render("PIPELINE"))

	// Stats
	stat := lipgloss.NewStyle().Width(m.width).Foreground(m.theme.Subtext).Padding(0, 1)
	lines = append(lines, stat.Render(fmt.Sprintf("%d total", m.metrics.Total)))
	lines = append(lines, stat.Render(fmt.Sprintf("%.1f avg", m.metrics.AvgScore)))

	// Separator
	lines = append(lines, lipgloss.NewStyle().Width(m.width).Foreground(m.theme.Overlay).Render(strings.Repeat("─", m.width)))

	// Filter items
	for i, item := range m.sections {
		isActive := i == activeItem
		itemStyle := lipgloss.NewStyle().Width(m.width).Padding(0, 1)
		
		if isActive {
			itemStyle = itemStyle.Background(m.theme.Overlay).Foreground(m.theme.Blue).Bold(true)
		} else {
			itemStyle = itemStyle.Foreground(m.theme.Text)
		}
		
		if item.Selected {
			itemStyle = itemStyle.Foreground(m.theme.Green)
		}

		dot := lipgloss.NewStyle().Foreground(item.Color).Render("●")
		count := ""
		if item.Count > 0 {
			count = fmt.Sprintf(" %d", item.Count)
		}
		lines = append(lines, itemStyle.Render(fmt.Sprintf("%s %s%s", dot, item.Label, count)))
	}

	// Separator
	lines = append(lines, lipgloss.NewStyle().Width(m.width).Foreground(m.theme.Overlay).Render(strings.Repeat("─", m.width)))

	// Help
	help := lipgloss.NewStyle().Width(m.width).Foreground(m.theme.Subtext).Padding(0, 1)
	lines = append(lines, help.Render("↑↓: nav"))
	lines = append(lines, help.Render("Enter: select"))
	lines = append(lines, help.Render("Ctrl+B: hide"))

	// Pad to height
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	return strings.Join(lines[:m.height], "\n")
}
