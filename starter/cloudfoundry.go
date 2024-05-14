package starter

import (
	"fmt"
	"github.com/cloudfoundry-community/gautocloud"
	"github.com/cloudfoundry-community/gautocloud/cloudenv"
	"gopkg.in/yaml.v2"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const (
	cfLauncherName string = "launcher"
	procFile       string = "Procfile"
)

type CloudFoundry struct {
}

func (s CloudFoundry) StartCmd(env []string, _ string, stdOut, stdErr io.Writer) (*exec.Cmd, error) {
	lPath := s.launcherPath()
	wd, _ := os.Getwd()
	cmd := exec.Command(lPath, wd, s.getUserStartCommand(), "")
	cmd.Env = env
	cmd.Dir = filepath.Dir(wd)
	cmd.Stdout = stdOut
	cmd.Stderr = stdErr
	return cmd, nil
}

func (s CloudFoundry) Name() string {
	return cloudenv.CfCloudEnv{}.Name()
}

func (s CloudFoundry) Detect() bool {
	return s.Name() == gautocloud.CurrentCloudEnv().Name()
}

func (CloudFoundry) getUserStartCommand() string {
	b, err := os.ReadFile(procFile)
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

func (CloudFoundry) launcherPath() string {
	lName := cfLauncherName
	base := "/tmp"
	if runtime.GOOS == "windows" {
		base = "C:\\tmp"
		lName += ".exe"
	}
	path := filepath.Join(base, "lifecycle", lName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		wd, _ := os.Getwd()
		return filepath.Join(wd, lName)
	}

	return path
}

func (CloudFoundry) AppPort() int {
	return gautocloud.GetAppInfo().Port
}

func (s CloudFoundry) ProxyEnv(appPort int) map[string]string {
	sPort := fmt.Sprintf("%d", appPort)
	return map[string]string{
		"PORT":          sPort,
		"VCAP_APP_PORT": sPort,
	}
}
