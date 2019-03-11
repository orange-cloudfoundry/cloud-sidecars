package starter

import (
	"io"
	"os/exec"
)

type Starter interface {
	StartCmd(env []string, profileDir string, stdOut, stdErr io.Writer) (*exec.Cmd, error)
	Name() string
	ProxyEnv(appPort int) map[string]string
	AppPort() int
	Detect() bool
}

func Retrieve() []Starter {
	return []Starter{
		BuildpackIO{},
		CloudFoundry{},
		Local{},
	}
}
