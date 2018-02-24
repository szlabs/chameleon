package lib

import (
	"errors"
	"fmt"
	"log"
	"registry-factory/client"
)

//Packer ...
type Packer struct {
	hostOn    string
	docker    *client.DockerClient
	harbor    string
	namespace string //registry namespace , use public 'library' now
}

//NewPacker ...
func NewPacker(dockerdHost string, hPort int, harborHost string) *Packer {
	dHost := ""
	if hPort > 0 {
		dHost = fmt.Sprintf("tcp://%s:%d", dockerdHost, hPort)
	}

	return &Packer{
		hostOn: dockerdHost,
		docker: &client.DockerClient{
			Host: dHost,
		},
		namespace: "library",
		harbor:    harborHost,
	}
}

//Build ...
func (p *Packer) Build(baseContainer string, image, tag string) error {
	if len(baseContainer) == 0 {
		return errors.New("empty base container")
	}

	newTag := tag
	if len(newTag) == 0 {
		newTag = "latest"
	}

	fullNamespace := fmt.Sprintf("%s/%s/%s", p.harbor, p.namespace, image)
	if err := p.docker.Commit(baseContainer, fullNamespace, newTag); err != nil {
		return err
	}

	//login
	if err := p.docker.Login("admin", "Harbor12345", p.harbor); err != nil {
		return err
	}
	backendImage := fmt.Sprintf("%s:%s", fullNamespace, newTag)
	if err := p.docker.Push(backendImage); err != nil {
		return err
	}

	//Just try to remove local image
	if err := p.docker.RMImage(backendImage); err != nil {
		log.Printf("rm image error: %s\n", err)
	}

	return nil
}

//BuildLocal ...
func (p *Packer) BuildLocal(baseContainer string, image, tag string) error {
	if len(baseContainer) == 0 {
		return errors.New("empty base container")
	}

	newTag := tag
	if len(newTag) == 0 {
		newTag = "latest"
	}
	return p.docker.Commit(baseContainer, image, newTag)
}
