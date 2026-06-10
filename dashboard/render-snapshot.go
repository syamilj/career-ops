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

	// Parse width from arg or default to 180 (wide terminal)
	width := 180
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &width)
	}
	height := 50
	if len(os.Args) > 3 {
		fmt.Sscanf(os.Args[3], "%d", &height)
	}

	apps := data.ParseApplications(careerOpsPath)
	metrics := data.ComputeMetrics(apps)

	t := theme.NewTheme("auto")
	pm := screens.NewPipelineModel(t, apps, metrics, careerOpsPath, width, height)

	// Load all reports
	for _, app := range apps {
		if app.ReportPath == "" {
			continue
		}
		archetype, tldr, remote, comp, domain, seniority := data.LoadReportSummary(careerOpsPath, app.ReportPath)
		pm.EnrichReport(app.ReportPath, archetype, tldr, remote, comp, domain, seniority)
	}

	// Render at different widths
	for _, w := range []int{width} {
		pm.Resize(w, height)
		output := pm.View()
		snapFile := "/Users/syamiljihad/CODE/career-ops/output/snapshot-" + fmt.Sprintf("%d", w) + ".txt"
		err := os.WriteFile(snapFile, []byte(output), 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR writing %s: %v\n", snapFile, err)
		} else {
			fmt.Printf("Wrote %s (%dx%d, %d bytes)\n", snapFile, w, height, len(output))
		}
	}
}
