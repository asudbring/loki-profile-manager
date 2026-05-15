package store

const (
	StoreDirName    = "loki"
	RegistryVersion = 1
)

type ProviderType string

const (
	ProviderOneDrive         ProviderType = "onedrive"
	ProviderOneDriveBusiness ProviderType = "onedrive-business"
	ProviderOneDrivePersonal ProviderType = "onedrive-personal"
	ProviderDropbox          ProviderType = "dropbox"
	ProviderManual           ProviderType = "manual"
)

type ProviderCandidate struct {
	Provider  ProviderType `json:"provider"`
	Path      string       `json:"path"`
	StorePath string       `json:"store_path"`
	Source    string       `json:"source"`
	Exists    bool         `json:"exists"`
}

type DiscoveryOptions struct {
	GOOS       string
	HomeDir    string
	ManualPath string
	Env        func(string) string
	Exists     func(string) bool
	Glob       func(string) ([]string, error)
	ReadFile   func(string) ([]byte, error)
}

type Layout struct {
	Root         string
	RegistryDir  string
	MachinesFile string
	MachinesDir  string
	ProfilesDir  string
	ConflictsDir string
	SnapshotsDir string
	LogsDir      string
}

type ValidationResult struct {
	Valid   bool     `json:"valid"`
	Missing []string `json:"missing"`
}

type InspectionResult struct {
	Exists  bool     `json:"exists"`
	Empty   bool     `json:"empty"`
	IsDir   bool     `json:"is_dir"`
	Valid   bool     `json:"valid"`
	Missing []string `json:"missing"`
}

type EnsureResult struct {
	Layout  Layout   `json:"layout"`
	Created bool     `json:"created"`
	Valid   bool     `json:"valid"`
	Missing []string `json:"missing"`
}
