package starter

import (
	"io"
	"os/exec"
)

type Starter interface {
	StartCmd(env []string, profileDir string, stdOut, stdErr io.Writer) (*exec.Cmd, error)
	CloudEnvName() string
	ProxyEnv(appPort int) map[string]string
	AppPort() int
}

func Retrieve() []Starter {
	return []Starter{
		CloudFoundry{},
		Local{},
	}
}
