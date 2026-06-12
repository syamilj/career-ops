//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/santifer/career-ops/dashboard/internal/data"
	"github.com/santifer/career-ops/dashboard/internal/theme"
	"github.com/santifer/career-ops/dashboard/internal/ui/screens"
)

func main() {
	careerOpsPath := ".."
	if len(os.Args) > 1 {
		careerOpsPath = os.Args[1]
	}

	width := 160
	height := 45

	apps := data.ParseApplications(careerOpsPath)
	metrics := data.ComputeMetrics(apps)

	t := theme.NewTheme("auto")
	layout := screens.NewLayoutModel(t, apps, metrics, careerOpsPath, width, height)

	for _, app := range apps {
		if app.ReportPath != "" {
			archetype, tldr, remote, comp, domain, seniority := data.LoadReportSummary(careerOpsPath, app.ReportPath)
			layout.Pipeline().EnrichReport(app.ReportPath, archetype, tldr, remote, comp, domain, seniority)
		}
	}

	// Layout snapshot
	layout.Resize(width, height)
	output := layout.View()
	os.MkdirAll("output", 0754)
	os.WriteFile("output/snapshot-layout.txt", []byte(output), 0644)
	fmt.Printf("Wrote layout snapshot (%dx%d)\n", width, height)

	// Viewer snapshot
	for _, app := range apps {
		if app.ReportPath != "" {
			fullPath := careerOpsPath + "/" + app.ReportPath
			title := app.Company + " — " + app.Role
			viewer := screens.NewViewerModel(t, fullPath, title, width, height)
			viewerOutput := viewer.View()
			os.WriteFile("output/snapshot-viewer.txt", []byte(viewerOutput), 0644)
			fmt.Printf("Wrote viewer snapshot (%dx%d)\n", width, height)
			break
		}
	}
}
