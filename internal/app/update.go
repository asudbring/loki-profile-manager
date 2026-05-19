package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/db"
)

const (
	UpdatePackageName       = "@asudbring/loki-profile-manager"
	updateCheckCacheKey     = "update_check"
	updateCheckCacheTTL     = 24 * time.Hour
	UpdateDisableEnvVar     = "LOKI_NO_UPDATE_CHECK"
	updateLatestPackageSpec = "@asudbring/loki-profile-manager@latest"
)

type UpdateCommandRunner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, name string, args ...string) (UpdateCommandResult, error)
}

type UpdateCommandResult struct {
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
}

type execUpdateRunner struct{}

func defaultUpdateRunner() UpdateCommandRunner {
	return execUpdateRunner{}
}

func (execUpdateRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (execUpdateRunner) Run(ctx context.Context, name string, args ...string) (UpdateCommandResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return UpdateCommandResult{Stdout: stdout.String(), Stderr: stderr.String()}, err
}

type UpdateCheckRequest struct {
	Force bool
	Now   func() time.Time
}

type UpdateCheckResult struct {
	Checked        bool   `json:"checked"`
	FromCache      bool   `json:"from_cache"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version,omitempty"`
	Available      bool   `json:"available"`
	Message        string `json:"message,omitempty"`
	SkippedReason  string `json:"skipped_reason,omitempty"`
}

type UpdateRequest struct {
	Now func() time.Time
}

type UpdateResult struct {
	CurrentVersion string              `json:"current_version"`
	LatestVersion  string              `json:"latest_version,omitempty"`
	Updated        bool                `json:"updated"`
	Command        []string            `json:"command,omitempty"`
	Output         UpdateCommandResult `json:"output,omitempty"`
	Message        string              `json:"message,omitempty"`
}

type updateCheckCache struct {
	CheckedAt     string `json:"checked_at"`
	LatestVersion string `json:"latest_version"`
}

func (s *Service) CheckForUpdate(ctx context.Context, req UpdateCheckRequest) (UpdateCheckResult, error) {
	if s == nil {
		return UpdateCheckResult{}, fmt.Errorf("update check: service is nil")
	}
	current := strings.TrimSpace(Version)
	result := UpdateCheckResult{CurrentVersion: current}
	if isDevelopmentVersion(current) {
		result.SkippedReason = "development version"
		return result, nil
	}
	now := time.Now
	if req.Now != nil {
		now = req.Now
	}
	if !req.Force {
		if cached, ok, err := s.readUpdateCheckCache(ctx); err != nil {
			return result, err
		} else if ok {
			checkedAt, parseErr := time.Parse(time.RFC3339, cached.CheckedAt)
			if parseErr == nil && now().Sub(checkedAt) < updateCheckCacheTTL {
				result.Checked = true
				result.FromCache = true
				result.LatestVersion = cached.LatestVersion
				result.Available = isVersionNewer(current, cached.LatestVersion)
				if result.Available {
					result.Message = updateNoticeMessage(result.LatestVersion)
				}
				return result, nil
			}
		}
	}
	latest, err := s.latestNPMVersion(ctx)
	if err != nil {
		return result, err
	}
	result.Checked = true
	result.LatestVersion = latest
	result.Available = isVersionNewer(current, latest)
	if result.Available {
		result.Message = updateNoticeMessage(latest)
	}
	if err := s.writeUpdateCheckCache(ctx, updateCheckCache{CheckedAt: now().UTC().Format(time.RFC3339), LatestVersion: latest}); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) Update(ctx context.Context, req UpdateRequest) (UpdateResult, error) {
	if s == nil {
		return UpdateResult{}, fmt.Errorf("update: service is nil")
	}
	current := strings.TrimSpace(Version)
	result := UpdateResult{CurrentVersion: current}
	if _, err := s.updateRunner.LookPath("npm"); err != nil {
		return result, fmt.Errorf("update: npm not found; install Node.js/npm or run `npm install -g %s` manually", UpdatePackageName)
	}
	if check, err := s.CheckForUpdate(ctx, UpdateCheckRequest{Force: true, Now: req.Now}); err == nil {
		result.LatestVersion = check.LatestVersion
	}
	args := []string{"install", "-g", updateLatestPackageSpec}
	output, err := s.updateRunner.Run(ctx, "npm", args...)
	result.Command = append([]string{"npm"}, args...)
	result.Output = output
	if err != nil {
		detail := firstLine(firstNonEmptyString(output.Stderr, output.Stdout))
		if detail != "" {
			return result, fmt.Errorf("update: npm install failed: %w: %s", err, detail)
		}
		return result, fmt.Errorf("update: npm install failed: %w", err)
	}
	result.Updated = true
	result.Message = fmt.Sprintf("Updated Loki to %s. Restart your shell if `loki --version` still shows the old version.", firstNonEmptyString(result.LatestVersion, "latest"))
	return result, nil
}

func (s *Service) latestNPMVersion(ctx context.Context) (string, error) {
	if _, err := s.updateRunner.LookPath("npm"); err != nil {
		return "", fmt.Errorf("update check: npm not found")
	}
	output, err := s.updateRunner.Run(ctx, "npm", "view", UpdatePackageName, "version", "--silent")
	if err != nil {
		return "", fmt.Errorf("update check: npm view failed: %w", err)
	}
	latest := strings.TrimSpace(output.Stdout)
	if latest == "" {
		latest = strings.TrimSpace(output.Stderr)
	}
	latest = firstLine(latest)
	if !validComparableVersion(latest) {
		return "", fmt.Errorf("update check: npm returned invalid version %q", latest)
	}
	return latest, nil
}

func (s *Service) readUpdateCheckCache(ctx context.Context) (updateCheckCache, bool, error) {
	value, ok, err := db.GetKV(ctx, s.database, updateCheckCacheKey)
	if err != nil || !ok || strings.TrimSpace(value) == "" {
		return updateCheckCache{}, false, err
	}
	var cached updateCheckCache
	if err := json.Unmarshal([]byte(value), &cached); err != nil {
		return updateCheckCache{}, false, nil
	}
	if cached.CheckedAt == "" || cached.LatestVersion == "" {
		return updateCheckCache{}, false, nil
	}
	return cached, true, nil
}

func (s *Service) writeUpdateCheckCache(ctx context.Context, cached updateCheckCache) error {
	content, err := json.Marshal(cached)
	if err != nil {
		return err
	}
	return db.SetKV(ctx, s.database, updateCheckCacheKey, string(content))
}

func isDevelopmentVersion(version string) bool {
	version = strings.TrimSpace(strings.ToLower(version))
	return version == "" || version == "dev" || version == "0.0.0-dev" || strings.Contains(version, "dev")
}

func updateNoticeMessage(latest string) string {
	return fmt.Sprintf("Newer Loki version available: %s. Run `loki update` to install the latest npm build.", latest)
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.IndexAny(value, "\r\n"); idx >= 0 {
		return strings.TrimSpace(value[:idx])
	}
	return value
}

var comparableVersionRE = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

type comparableVersion struct {
	parts      [3]int
	prerelease []string
}

func validComparableVersion(version string) bool {
	_, ok := parseComparableVersion(version)
	return ok
}

func isVersionNewer(current, latest string) bool {
	currentVersion, currentOK := parseComparableVersion(current)
	latestVersion, latestOK := parseComparableVersion(latest)
	if !currentOK || !latestOK {
		return false
	}
	return compareComparableVersions(latestVersion, currentVersion) > 0
}

func parseComparableVersion(version string) (comparableVersion, bool) {
	var out comparableVersion
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if !comparableVersionRE.MatchString(version) {
		return out, false
	}
	withoutBuild := version
	if idx := strings.Index(withoutBuild, "+"); idx >= 0 {
		withoutBuild = withoutBuild[:idx]
	}
	base := withoutBuild
	if idx := strings.Index(base, "-"); idx >= 0 {
		out.prerelease = strings.Split(base[idx+1:], ".")
		base = base[:idx]
	}
	parts := strings.Split(base, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return out, false
		}
		out.parts[i] = value
	}
	return out, true
}

func compareComparableVersions(left, right comparableVersion) int {
	for i := 0; i < 3; i++ {
		if left.parts[i] > right.parts[i] {
			return 1
		}
		if left.parts[i] < right.parts[i] {
			return -1
		}
	}
	return comparePrerelease(left.prerelease, right.prerelease)
}

func comparePrerelease(left, right []string) int {
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	if len(left) == 0 {
		return 1
	}
	if len(right) == 0 {
		return -1
	}
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for i := 0; i < limit; i++ {
		if cmp := comparePrereleaseIdentifier(left[i], right[i]); cmp != 0 {
			return cmp
		}
	}
	if len(left) > len(right) {
		return 1
	}
	if len(left) < len(right) {
		return -1
	}
	return 0
}

func comparePrereleaseIdentifier(left, right string) int {
	leftNumber, leftIsNumber := parseNumericPrereleaseIdentifier(left)
	rightNumber, rightIsNumber := parseNumericPrereleaseIdentifier(right)
	switch {
	case leftIsNumber && rightIsNumber:
		if leftNumber > rightNumber {
			return 1
		}
		if leftNumber < rightNumber {
			return -1
		}
		return 0
	case leftIsNumber:
		return -1
	case rightIsNumber:
		return 1
	default:
		return strings.Compare(left, right)
	}
}

func parseNumericPrereleaseIdentifier(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	number, err := strconv.Atoi(value)
	return number, err == nil
}
