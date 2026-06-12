package screens

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/santifer/career-ops/dashboard/internal/model"
	"github.com/santifer/career-ops/dashboard/internal/theme"
)

// LayoutModel implements split-pane: sidebar (fixed 22 cols) | pipeline (rest).
type LayoutModel struct {
	sidebar         SidebarModel
	pipeline        PipelineModel
	sidebarWidth    int
	width           int
	height          int
	theme           theme.Theme
	focusOnSidebar  bool
}

func NewLayoutModel(t theme.Theme, apps []model.CareerApplication, metrics model.PipelineMetrics, careerOpsPath string, width, height int) LayoutModel {
	sw := 22
	if width < 80 {
		sw = 16
	}

	sidebar := NewSidebarModel(t, apps, metrics, sw, height)
	pipeline := NewPipelineModel(t, apps, metrics, careerOpsPath, width-sw, height)
	pipeline.SetCompact(true)

	return LayoutModel{
		sidebar:      sidebar,
		pipeline:     pipeline,
		sidebarWidth: sw,
		width:        width,
		height:       height,
		theme:        t,
	}
}

func (m LayoutModel) Init() tea.Cmd { return nil }

func (m *LayoutModel) Resize(width, height int) {
	m.width = width
	m.height = height
	sw := 22
	if width < 80 {
		sw = 16
	}
	m.sidebarWidth = sw
	m.sidebar.Resize(sw, height)
	m.pipeline.Resize(width-sw, height)
}

func (m LayoutModel) Update(msg tea.Msg) (LayoutModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			m.focusOnSidebar = !m.focusOnSidebar
			return m, nil
		case "ctrl+b":
			if m.sidebarCollapsed() {
				m.sidebar.ToggleCollapse()
				m.sidebarWidth = 22
				if m.width < 80 {
					m.sidebarWidth = 16
				}
			} else {
				m.sidebar.ToggleCollapse()
				m.sidebarWidth = 3
			}
			m.pipeline.Resize(m.width-m.sidebarWidth, m.height)
			return m, nil
		case "esc":
			if m.focusOnSidebar {
				m.focusOnSidebar = false
				return m, nil
			}
		}
	case tea.MouseMsg:
		if msg.Type == tea.MouseWheelUp || msg.Type == tea.MouseWheelDown {
			if m.focusOnSidebar {
				newSidebar, cmd := m.sidebar.Update(msg)
				m.sidebar = newSidebar
				return m, cmd
			}
			newPipeline, cmd := m.pipeline.Update(msg)
			m.pipeline = newPipeline
			return m, cmd
		}
	}

	if m.focusOnSidebar {
		newSidebar, cmd := m.sidebar.Update(msg)
		m.sidebar = newSidebar
		return m, cmd
	}
	newPipeline, cmd := m.pipeline.Update(msg)
	m.pipeline = newPipeline
	return m, cmd
}

func (m LayoutModel) sidebarCollapsed() bool {
	return m.sidebarWidth <= 3
}

func (m LayoutModel) View() string {
	sb := m.sidebar.View()
	pl := m.pipeline.View()
	return lipgloss.JoinHorizontal(lipgloss.Top, sb, pl)
}

func (m *LayoutModel) Pipeline() *PipelineModel    { return &m.pipeline }
func (m *LayoutModel) Sidebar() *SidebarModel      { return &m.sidebar }
func (m *LayoutModel) SetStatusMsg(msg string)      { m.pipeline.SetStatusMsg(msg) }
func (m *LayoutModel) PipelineCopy(p PipelineModel) { m.pipeline = p }
