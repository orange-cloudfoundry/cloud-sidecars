package starter

import (
	"fmt"
	"github.com/cloudfoundry-community/gautocloud"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	BpIoPathEnvVarKey string = "BUILDPACKS_IO_LAUNCHER_PATH"
)

type BuildpackIO struct {
}

func (s BuildpackIO) StartCmd(env []string, _ string, stdOut, stdErr io.Writer) (*exec.Cmd, error) {
	lPath := s.launcherPath()
	wd, _ := os.Getwd()
	cmd := exec.Command(lPath, s.getUserStartCommand())
	cmd.Env = env
	cmd.Dir = filepath.Dir(wd)
	cmd.Stdout = stdOut
	cmd.Stderr = stdErr
	return cmd, nil
}

func (s BuildpackIO) Name() string {
	return "buildpacksio"
}

func (BuildpackIO) getUserStartCommand() string {
	b, err := ioutil.ReadFile(procFile)
	if err != nil {
		return ""
	}
	startCommandS := struct {
		StartCommand string `yaml:"start"`
	}{}
	err = yaml.Unmarshal(b, &startCommandS)
	if err != nil {
		return ""
	}
	return startCommandS.StartCommand
}

func (s BuildpackIO) Detect() bool {
	return os.Getenv(BpIoPathEnvVarKey) != ""
}

func (BuildpackIO) launcherPath() string {
	return os.Getenv(BpIoPathEnvVarKey)
}

func (BuildpackIO) AppPort() int {
	return gautocloud.GetAppInfo().Port
}

func (s BuildpackIO) ProxyEnv(appPort int) map[string]string {
	sPort := fmt.Sprintf("%d", appPort)
	return map[string]string{
		"PORT":          sPort,
		"VCAP_APP_PORT": sPort,
	}
}
