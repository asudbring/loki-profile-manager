package tui

func (m Model) profilesView() string {
	lines := []string{
		titleStyle.Render("Profiles"),
		"",
		labelValue("Store", firstNonEmpty(m.catalog.StorePath, m.status.StorePath, "not configured")),
		labelValue("Catalog", formatCatalogSummary(m.catalog)),
	}
	if m.catalogErr != nil {
		lines = append(lines, "", errorStyle.Render("Profile catalog unavailable: "+m.catalogErr.Error()))
	} else if !m.status.Configured {
		lines = append(lines, "", mutedStyle.Render("Store not configured; profile catalog unavailable."))
	} else if len(m.catalog.Profiles) == 0 {
		lines = append(lines, "", mutedStyle.Render("No profiles found."))
	} else {
		lines = append(lines, "", subtitleStyle.Render("Profiles and buckets"))
		for _, profile := range m.catalog.Profiles {
			lines = append(lines, "- "+profile.Name)
			for _, bucket := range profile.Buckets {
				lines = append(lines, "  • "+bucket.Name)
			}
		}
	}
	lines = append(lines, "", helpStyle.Render("esc back • r refresh • q quit"))
	return frame(lines...)
}
