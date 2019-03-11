package sidecars

import (
	"fmt"
	"github.com/orange-cloudfoundry/cloud-sidecars/config"
	"github.com/orange-cloudfoundry/cloud-sidecars/starter"
	"github.com/orange-cloudfoundry/cloud-sidecars/utils"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

type ProcessFactory struct {
	errChan    chan error
	signalChan chan os.Signal
	wg         *sync.WaitGroup
	wd         string
	stdout     io.Writer
	stderr     io.Writer
	cStarter   starter.Starter
}

func NewProcessFactory(
	stdout, stderr io.Writer,
	cStarter starter.Starter,
	wd string) *ProcessFactory {
	return &ProcessFactory{
		errChan:    make(chan error, 100),
		signalChan: make(chan os.Signal, 100),
		wg:         &sync.WaitGroup{},
		stderr:     stderr,
		stdout:     stdout,
		wd:         wd,
		cStarter:   cStarter,
	}
}

func (f *ProcessFactory) WaitGroup() *sync.WaitGroup {
	return f.wg
}

func (f *ProcessFactory) ErrorChan() chan error {
	return f.errChan
}

func (f *ProcessFactory) SignalChan() chan os.Signal {
	return f.signalChan
}

func (f *ProcessFactory) FromStarter(env map[string]string, profileDir string) (*process, error) {
	cloudCmd, err := f.cStarter.StartCmd(
		utils.EnvMapToOsEnv(env),
		profileDir,
		f.stdout,
		f.stderr,
	)
	if err != nil {
		return nil, err
	}
	// set pgid for sending signal to child
	cloudCmd.SysProcAttr = utils.PgidSysProcAttr(cloudCmd.SysProcAttr)
	return &process{
		cmd:             cloudCmd,
		name:            "launcher",
		typeP:           "cloud",
		noInterrupt:     true,
		alwaysInterrupt: true,
		errChan:         f.errChan,
		signalChan:      f.signalChan,
		wg:              f.wg,
	}, nil
}

func (f *ProcessFactory) FromSidecar(sidecar *config.Sidecar, env map[string]string) (*process, error) {
	var err error
	wd := f.wd
	if sidecar.WorkDir != "" {
		wd = sidecar.WorkDir
	}
	if wd == "" {
		wd, _ = os.Getwd()
	}

	if _, err := os.Stat(wd); os.IsNotExist(err) {
		return nil, fmt.Errorf("Workdir '%s' doesn't exists.", wd)
	}

	args, err := TemplatingArgs(env, sidecar.Args...)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(SidecarExecPath(f.wd, sidecar), args...)
	cmd.Env = utils.EnvMapToOsEnv(env)
	cmd.Dir = wd
	// set pgid for sending signal to child
	cmd.SysProcAttr = utils.PgidSysProcAttr(nil)
	if !sidecar.NoLogPrefix {
		writerPrefix := fmt.Sprintf("[sidecar:%s]", sidecar.Name)
		err := PrefixCmdOutput(f.stdout, f.stderr, cmd, writerPrefix)
		if err != nil {
			return nil, err
		}
	} else {
		cmd.Stdout = f.stdout
		cmd.Stderr = f.stderr
	}
	return &process{
		cmd:         cmd,
		name:        sidecar.Name,
		typeP:       "sidecar",
		noInterrupt: sidecar.NoInterruptWhenStop,
		errChan:     f.errChan,
		signalChan:  f.signalChan,
		wg:          f.wg,
	}, nil
}

func SidecarExecPath(origWd string, sidecar *config.Sidecar) string {
	execPath := sidecar.Executable
	wd := origWd
	if wd == "" {
		wd, _ = os.Getwd()
	}
	if sidecar.ArtifactURI != "" {
		execPath = filepath.Join(SidecarDir(wd, sidecar.Name), execPath)
	}
	return execPath
}
