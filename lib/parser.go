package lib

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

//Parser ...
type Parser func(req *http.Request) (RequestMeta, error)

//NpmParser ...
func NpmParser(req *http.Request) (RequestMeta, error) {
	userAgent := req.Header.Get("User-Agent")
	if strings.Contains(userAgent, "npm/") {
		//Hit
		npmCmd := req.Header.Get("Referer")
		if len(npmCmd) > 0 {
			meta := RequestMeta{
				RegistryType: registryTypeNpm,
				RequestStage: requestStageRun,
				HasHit:       true,
				BoundPorts:   []int32{80},
			}
			commands := strings.Split(npmCmd, " ")
			if len(commands) > 0 {
				command := strings.TrimSpace(commands[0])
				if command == "login" ||
					command == "adduser" ||
					command == "add-user" {
					meta.RequestStage = requestStageSession
					meta.Image = "stevenzou/npm-registry"
					meta.Tag = "latest"
				} else if command == "publish" {
					meta.RequestStage = requestStagePack
					meta.Image = "stevenzou/npm-registry"
					meta.Tag = "latest"
				} else {
					meta.RequestStage = requestStageRun
					if len(commands) > 1 {
						moreInfo := strings.TrimSpace(commands[1])
						if strings.Contains(moreInfo, "@") {
							imageInfos := strings.Split(moreInfo, "@")
							if len(imageInfos) >= 2 {
								//SHOULD confirm if the image existing
								//or use the default base one
								meta.Image = strings.TrimSpace(imageInfos[0])
								meta.Tag = strings.TrimSpace(imageInfos[1])
							}
						}
					}

					if len(meta.Image) == 0 {
						meta.Image = "stevenzou/npm-registry"
						meta.Tag = "latest"
					}
				}

				return meta, nil
			}
		}
	}

	return RequestMeta{}, nil
}

//ParserChain ...
type ParserChain struct {
	head *parserWrapper
	tail *parserWrapper
}

//ParserWrapper ...
type parserWrapper struct {
	parser Parser
	next   *parserWrapper
}

//Parse ...
func (pc *ParserChain) Parse(req *http.Request) (RequestMeta, error) {
	if pc.head == nil {
		return RequestMeta{}, errors.New("no parsers")
	}

	var errs []string
	p := pc.head
	for p != nil && p.parser != nil {
		if meta, err := p.parser(req); err != nil {
			errs = append(errs, err.Error())
		} else {
			if meta.HasHit {
				return meta, nil
			}
		}

		//next
		p = p.next
	}

	//No hit
	return RequestMeta{}, fmt.Errorf("%s:%s", "no hit", strings.Join(errs, ";"))
}

//Init ...
func (pc *ParserChain) Init() error {
	pc.head = nil
	pc.tail = nil

	return pc.Register(NpmParser)
}

//Register ...
func (pc *ParserChain) Register(parser Parser) error {
	if parser == nil {
		return errors.New("nil parser")
	}

	if pc.head == nil {
		pc.head = &parserWrapper{parser, nil}
		pc.tail = pc.head

		return nil
	}

	pc.tail.next = &parserWrapper{parser, nil}
	pc.tail = pc.tail.next

	return nil
}
