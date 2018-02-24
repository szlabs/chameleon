package lib

const (
	registryTypeNpm  = "npm"
	registryTypeHelm = "helm"

	requestStageSession = "session"
	requestStagePack    = "pack"
	requestStageRun     = "runtime"
)

//RequestMeta ...
type RequestMeta struct {
	RegistryType string
	RequestStage string
	HasHit       bool
	Image        string
	Tag          string
	BoundPorts   []int32
}
