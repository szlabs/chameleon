package lib

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

//ProxyTarget ...
type ProxyTarget string

//SchedulerConfig ...
type SchedulerConfig struct {
	DockerHost  string
	HPort       int
	Harbor      string
	Namespace   string //use 'npm-registry' now
	HarborProto string
}

//Scheduler ...
type Scheduler struct {
	pool        *RuntimePool
	executor    *Executor
	packer      *Packer
	ctx         context.Context
	drivers     map[string]ScheduleDriver
	registryAPI string
	namespace   string
}

//NewScheduler ...
func NewScheduler(ctx context.Context, cfg SchedulerConfig) *Scheduler {
	return &Scheduler{
		pool:        NewRuntimePool(),
		executor:    NewExecutor(cfg.DockerHost, cfg.HPort, cfg.Harbor, cfg.Namespace),
		packer:      NewPacker(cfg.DockerHost, cfg.HPort, cfg.Harbor, cfg.Namespace),
		ctx:         ctx,
		registryAPI: fmt.Sprintf("%s://%s/api", cfg.HarborProto, cfg.Harbor),
		namespace:   cfg.Namespace,
	}
}

//Start ...
func (s *Scheduler) Start() {
	go func() {
		tk := time.Tick(30 * time.Second)
		for {
			select {
			case <-tk:
				//Garbage collection
				garbages := s.pool.Garbages()
				if len(garbages) > 0 {
					for _, v := range garbages {
						//Clear
						if err := s.executor.Destroy(v.ID); err != nil {
							log.Fatalf("garbage collection %s error: %s\n", v.ID, err)
						}
					}
				}
			case <-s.ctx.Done():
				log.Println("Scheduler stop")
				return
			}
		}
	}()

	s.drivers = make(map[string]ScheduleDriver)
	s.drivers[registryTypeNpm] = NewNpmScheduleDriver(s.registryAPI, s.namespace)
	s.drivers[registryTypePip] = NewPipScheduleDriver(s.registryAPI, "pip-project")

	log.Println("Scheduler is started")
}

//Schedule ...
func (s *Scheduler) Schedule(meta RequestMeta) (ServeEnvironment, error) {
	driver, ok := s.drivers[meta.RegistryType]
	if !ok {
		return ServeEnvironment{}, fmt.Errorf("registry type %s not support", meta.RegistryType)
	}

	policy := driver.Schedule(meta)
	if len(policy.ReuseIdentity) > 0 {
		key := fmt.Sprintf("%s:%s", meta.RegistryType, policy.ReuseIdentity)
		_, yes := s.pool.Index(key)
		if yes {
			r, err := s.pool.Use(key)
			if err != nil {
				return ServeEnvironment{}, err
			}
			log.Printf("Reuse %s\n", r.Target)
			if policy.Rebuild != nil {
				policy.Rebuild.BaseContainer = r.ID
			}
			return ServeEnvironment{
				Target:  r.Target,
				Rebuild: policy.Rebuild,
			}, nil
		}
	}

	//Create
	env, err := s.executor.Exec(policy)
	if err != nil {
		return ServeEnvironment{}, err
	}

	key := env.RuntimeID //Just for garbage collection
	if len(policy.ReuseIdentity) > 0 {
		key = policy.ReuseIdentity
	}
	key = fmt.Sprintf("%s:%s", meta.RegistryType, key)

	s.pool.Put(key, env.RuntimeID, env.Target)

	if policy.Rebuild != nil {
		policy.Rebuild.BaseContainer = env.RuntimeID
	}
	return ServeEnvironment{
		Target:  env.Target,
		Rebuild: policy.Rebuild,
	}, nil
}

//Rebuild ...
func (s *Scheduler) Rebuild(policy *BuildPolicy) error {
	if policy == nil {
		return errors.New("nil build policy")
	}

	if len(policy.Image) == 0 || len(policy.Tag) == 0 {
		return errors.New("target image or tag is invalid")
	}

	if len(policy.BaseContainer) == 0 {
		return errors.New("no base container for build")
	}

	if policy.NeedPush {
		return s.packer.Build(policy.BaseContainer, policy.Image, policy.Tag)
	}

	return s.packer.BuildLocal(policy.BaseContainer, policy.Image, policy.Tag)
}

//ServeEnvironment ...
type ServeEnvironment struct {
	Target  ProxyTarget
	Rebuild *BuildPolicy
}

//SchedulePolicy ...
type SchedulePolicy struct {
	Image         string
	Tag           string
	UseHub        bool
	ReuseIdentity string
	BoundPorts    []int
	Rebuild       *BuildPolicy
	EnvVars       map[string]string
}

//BuildPolicy ...
type BuildPolicy struct {
	BaseContainer string `json:"base_container"`
	Image         string `json:"image"`
	Tag           string `json:"tag"`
	NeedPush      bool   `json:"need_push"`
}

//Encode ...
func (bp *BuildPolicy) Encode() (string, error) {
	bytes, err := json.Marshal(bp)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(bytes), nil
}

//Decode ...
func (bp *BuildPolicy) Decode(data string) error {
	bytes, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, bp)
}

//ScheduleDriver ...
type ScheduleDriver interface {
	//Schedule ...
	Schedule(meta RequestMeta) *SchedulePolicy
}

//PipScheduleDriver ...
type PipScheduleDriver struct {
	registryAPI       string
	registryNamespace string
	httpClient        *http.Client
}

//NewNpmScheduleDriver ...
func NewPipScheduleDriver(registryAPI, registryNamespace string) *PipScheduleDriver {
	return &PipScheduleDriver{
		registryAPI:       registryAPI,
		registryNamespace: registryNamespace,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}
}

//Schedule ...
func (psd *PipScheduleDriver) Schedule(meta RequestMeta) *SchedulePolicy {
	if !meta.HasHit || meta.RegistryType != registryTypePip {
		return nil
	}
	if meta.Metadata["command"] == "install" {
		//TODO: change
		image := fmt.Sprintf("pip-project/pypi-%s", meta.Metadata["package"])
		//Default policy
		policy := &SchedulePolicy{
			Image:         image,
			Tag:           "dev",
			BoundPorts:    []int{80},
			ReuseIdentity: meta.Metadata["package"],
			EnvVars:       map[string]string{"PYPI_EXTRA": "--disable-fallback", "PYPI_ROOT": "/pypi"},
		}
		return policy

	} else {
		log.Printf("Unknown command for pip package: %s\n", meta.Metadata["command"])
	}

	return nil
}

//NpmScheduleDriver ...
type NpmScheduleDriver struct {
	registryAPI       string
	registryNamespace string
	httpClient        *http.Client
}

//NewNpmScheduleDriver ...
func NewNpmScheduleDriver(registryAPI, registryNamespace string) *NpmScheduleDriver {
	return &NpmScheduleDriver{
		registryAPI:       registryAPI,
		registryNamespace: registryNamespace,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}
}

//Schedule ...
func (nsd *NpmScheduleDriver) Schedule(meta RequestMeta) *SchedulePolicy {
	if !meta.HasHit || len(meta.Metadata) == 0 || meta.RegistryType != registryTypeNpm {
		return nil
	}

	//Default policy
	policy := &SchedulePolicy{
		Image:      "stevenzou/npm-registry",
		Tag:        "latest",
		UseHub:     true,
		BoundPorts: []int{80},
		Rebuild: &BuildPolicy{
			Image: "stevenzou/npm-registry",
			Tag:   "latest",
		},
	}

	//If has reuseIdentity
	session := meta.Metadata["session"]
	if len(session) > 0 {
		policy.ReuseIdentity = session
	}

	//Special cases
	requestPath := meta.Metadata["path"]
	command := meta.Metadata["command"]
	if command == "view" || command == "install" {
		extraInfo := meta.Metadata["extra"]
		repo := strings.TrimPrefix(requestPath, "/")
		tag := strings.TrimSpace(strings.TrimPrefix(extraInfo, repo+"@"))
		if checkImageExisting(nsd.registryAPI, nsd.registryNamespace, repo, tag, nsd.httpClient) {
			policy.Image = repo
			policy.Tag = tag
			policy.UseHub = false
		}
		policy.Rebuild = nil
	}

	if command == "login" || command == "adduser" || command == "add-user" {
		if strings.Contains(requestPath, "org.couchdb.user:") &&
			!strings.Contains(requestPath, "/-rev/") {
			policy.Rebuild = nil
		}
	}

	if command == "publish" {
		repo := strings.TrimPrefix(requestPath, "/")
		tag := meta.Metadata["extra"]
		log.Printf("===Publishing===: %s@%s", repo, tag)
		if checkImageExisting(nsd.registryAPI, nsd.registryNamespace, repo, tag, nsd.httpClient) {
			policy.Image = repo
			policy.Tag = tag
			policy.UseHub = false
		}
		policy.Rebuild.Image = strings.TrimPrefix(requestPath, "/")
		policy.Rebuild.Tag = meta.Metadata["extra"]
		policy.Rebuild.NeedPush = true
	}

	return policy
}

func checkImageExisting(registryAPI, registryNamespace, image, tag string, client *http.Client) bool {
	url := fmt.Sprintf("%s%s%s%s%s%s%s", registryAPI, "/repositories/", registryNamespace, "/", image, "/tags/", tag)
	resp, err := client.Get(url)
	if err == nil {
		if resp.StatusCode == http.StatusOK {
			log.Printf("Image %s:%s existing\n", image, tag)
			return true
		}

		log.Printf("Image %s:%s not existing\n", image, tag)
		return false
	}

	log.Printf("Failed to Check image %s:%s existing: %s\n", image, tag, err)
	return false
}
