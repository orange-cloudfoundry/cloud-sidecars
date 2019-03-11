package starter

import (
	"fmt"
	"github.com/cloudfoundry-community/gautocloud"
	"github.com/cloudfoundry-community/gautocloud/cloudenv"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
)

const launcher = `
set -e
cd "$1"
if [ -n "$(ls $2/* 2> /dev/null)" ]; then
  for env_file in $2/*; do
    source $env_file
  done
fi
if [ -n "$(ls .profile.d/* 2> /dev/null)" ]; then
  for env_file in .profile.d/*; do
    source $env_file
  done
fi
if [ -f .profile ]; then
  source .profile
fi
shift
shift
bash -c "$@"
`

type Local struct {
}

func (s Local) StartCmd(env []string, profileDir string, stdOut, stdErr io.Writer) (*exec.Cmd, error) {
	wd, _ := os.Getwd()
	cmd := exec.Command("bash", "-c", launcher, os.Args[0], wd, profileDir, s.getUserStartCommand())
	cmd.Env = env
	cmd.Dir = wd
	cmd.Stdout = stdOut
	cmd.Stderr = stdErr
	return cmd, nil
}

func (Local) getUserStartCommand() string {
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

func (Local) Name() string {
	return cloudenv.LocalCloudEnv{}.Name()
}

func (s Local) Detect() bool {
	return s.Name() == gautocloud.CurrentCloudEnv().Name()
}

func (s Local) ProxyEnv(appPort int) map[string]string {
	sPort := fmt.Sprintf("%d", appPort)
	return map[string]string{
		"PROXY_APP_PORT": sPort,
		"PORT":           sPort,
		"VCAP_APP_PORT":  sPort,
	}
}

func (Local) AppPort() int {
	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		return 8080
	}
	return port
}
