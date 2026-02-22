package build

// OutputMode describes what the build produces.
type OutputMode string

const (
	OutputImage OutputMode = "image" // container image (push to registry or load to daemon)
	OutputLocal OutputMode = "local" // extract files to local filesystem
	OutputTar   OutputMode = "tar"   // export as tarball
)

// BuildPlan is the resolved execution plan for a build.
type BuildPlan struct {
	Steps []BuildStep
}

// BuildStep is a single build invocation.
type BuildStep struct {
	Name       string
	Dockerfile string
	Context    string
	Target     string
	Platforms  []string
	BuildArgs  map[string]string
	Tags       []string
	Output     OutputMode
	Extract    []ExtractRule    // artifact mode only
	Registries []RegistryTarget // image mode only
	Load       bool             // --load into daemon
	Push       bool             // --push to registries
	SavePath   string           // save image tarball here after build (for security scanning)
}

// ExtractRule defines a file to extract from a build container.
type ExtractRule struct {
	From string // path inside the container
	To   string // local destination path
}

// RegistryTarget is a resolved registry push destination.
type RegistryTarget struct {
	URL         string
	Path        string
	Tags        []string
	Credentials string // env var prefix for auth (e.g., "DOCKERHUB" → DOCKERHUB_USER/DOCKERHUB_PASS)
	Provider    string // registry vendor: dockerhub, ghcr, gitlab, jfrog, harbor, quay, generic
}
