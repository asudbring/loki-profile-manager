package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/skills"
)

func isSkillDir(path string) bool {
	info, err := os.Stat(filepath.Join(path, "SKILL.md"))
	return err == nil && !info.IsDir()
}

func validateSkillItem(item Item) error {
	if !item.IsSkill {
		return nil
	}
	result := skills.ValidateFolder(item.SourcePath)
	if result.Valid {
		return nil
	}
	parts := make([]string, 0, len(result.Issues))
	for _, issue := range result.Issues {
		parts = append(parts, issue.Code+": "+issue.Message)
	}
	return fmt.Errorf("validate skill %s: %s", item.SourcePath, strings.Join(parts, "; "))
}

func hashPath(path string) (string, error) {
	return activation.HashPath(path)
}
