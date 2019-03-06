package starter

import (
	"fmt"
	"github.com/cloudfoundry-community/gautocloud"
	"github.com/cloudfoundry-community/gautocloud/cloudenv"
	"github.com/orange-cloudfoundry/cloud-sidecars/utils"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const (
	launcherName string = "launcher"
	procFile     string = "Procfile"
)

type CloudFoundry struct {
}

func (s CloudFoundry) StartCmd(env []string, _ string, stdOut, stdErr io.Writer) (*exec.Cmd, error) {
	lPath := s.launcherPath()
	wd, _ := os.Getwd()
	cmd := exec.Command(lPath, wd, s.getUserStartCommand(), "")
	cmd.Env = env
	cmd.Dir = filepath.Dir(wd)
	// set pgid for sending signal to child
	cmd.SysProcAttr = utils.PgidSysProcAttr()
	cmd.Stdout = stdOut
	cmd.Stderr = stdErr
	return cmd, nil
}

func (s CloudFoundry) CloudEnvName() string {
	return cloudenv.CfCloudEnv{}.Name()
}

func (CloudFoundry) getUserStartCommand() string {
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

func (CloudFoundry) launcherPath() string {
	lName := launcherName
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
