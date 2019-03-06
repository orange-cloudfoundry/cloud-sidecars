package sidecars

import (
	"fmt"
	"github.com/gliderlabs/sigil"
	"github.com/olekukonko/tablewriter"
	"github.com/orange-cloudfoundry/cloud-sidecars/config"
	"github.com/orange-cloudfoundry/cloud-sidecars/starter"
	"github.com/orange-cloudfoundry/cloud-sidecars/utils"
	log "github.com/sirupsen/logrus"
	"gopkg.in/alessio/shellescape.v1"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
)

func init() {
	sigil.PosixPreprocess = true
}

const (
	ProxyAppPortEnvKey = "PROXY_APP_PORT"
	AppPortEnvKey      = "SIDECAR_APP_PORT"
	PathSidecarsWd     = ".sidecars"
)

type Launcher struct {
	sConfig    config.SidecarsConfig
	cStarter   starter.Starter
	profileDir string
	stdout     io.Writer
	stderr     io.Writer
	appPort    int
}

func NewLauncher(
	sConfig config.SidecarsConfig,
	cStarter starter.Starter,
	profileDir string,
	stdout, stderr io.Writer,
	defaultAppPort int,
) *Launcher {
	var appPort int
	if cStarter != nil && !sConfig.NoStarter {
		appPort = cStarter.AppPort()
	}
	if appPort == 0 {
		appPort = sConfig.AppPort
	}
	if appPort == 0 {
		appPort = defaultAppPort
	}
	return &Launcher{
		sConfig:    sConfig,
		cStarter:   cStarter,
		profileDir: profileDir,
		stdout:     stdout,
		stderr:     stderr,
		appPort:    appPort,
	}
}

func (l Launcher) ShowSidecarsSha1() error {
	table := tablewriter.NewWriter(l.stdout)
	table.SetHeader([]string{"Sidecar Name", "Sha1"})
	for _, sidecar := range l.sConfig.Sidecars {
		if sidecar.ArtifactURI == "" {
			table.Append([]string{sidecar.Name, "-"})
			continue
		}
		s, err := ZipperSess(sidecar.ArtifactURI, sidecar.ArtifactType)
		if err != nil {
			return NewSidecarError(sidecar, err)
		}
		sha1, err := s.Sha1()
		if err != nil {
			return NewSidecarError(sidecar, err)
		}
		table.Append([]string{sidecar.Name, sha1})
	}
	table.Render()
	return nil
}

func (l Launcher) Setup() error {
	entryG := log.WithField("component", "Launcher").WithField("command", "staging")
	entryG.Infof("Setup sidecars ...")
	appEnv := make(map[string]string)
	err := os.MkdirAll(l.profileDir, 0755)
	if err != nil {
		return err
	}
	appPort := l.appPort
	for _, sidecar := range l.sConfig.Sidecars {
		entry := entryG.WithField("sidecar", sidecar.Name)
		entry.Infof("Setup ...")
		appEnvUnTpl, err := l.templatingEnv(appEnv, sidecar.AppEnv)
		if err != nil {
			return err
		}
		appEnv = utils.MergeEnv(appEnv, appEnvUnTpl)
		if sidecar.ArtifactURI == "" {
			continue
		}
		if sidecar.IsRproxy {
			appPort++
		}
		err = l.DownloadArtifacts(false)
		if err != nil {
			return err
		}
		if sidecar.ProfileD != "" {
			entry.Infof("Writing profiled file ...")
			err := ioutil.WriteFile(
				filepath.Join(l.profileDir, sidecar.Name+".sh"),
				[]byte(sidecar.ProfileD), 0755)
			if err != nil {
				return err
			}
			entry.Infof("Finished writing profiled file.")
		}

		entry.Infof("Finished setup.")
	}
	entryG.Infof("Finished setup sidecars.")
	if l.cStarter == nil || l.sConfig.NoStarter {
		return nil
	}
	if appPort != l.appPort {
		appEnv = utils.MergeEnv(appEnv, l.cStarter.ProxyEnv(appPort))
		appEnv = utils.MergeEnv(appEnv, map[string]string{
			AppPortEnvKey: strconv.Itoa(l.appPort),
		})
	}
	entryG.WithField("starter", l.cStarter.CloudEnvName()).Info("Adding starter.sh profile")
	profileLaunch := ""
	for k, v := range appEnv {
		profileLaunch += fmt.Sprintf("export %s=%s\n", k, shellescape.Quote(v))
	}
	err = ioutil.WriteFile(
		filepath.Join(l.profileDir, "starter.sh"),
		[]byte(profileLaunch), 0755)
	if err != nil {
		return err
	}
	entryG.WithField("starter", l.cStarter.CloudEnvName()).Info("Finished adding starter.sh profile")
	return nil
}

func (l Launcher) DownloadArtifacts(forceDl bool) error {
	entryG := log.WithField("component", "Launcher").WithField("command", "download_artifact")
	entryG.Info("Start downloading artifacts from sidecars ...")
	for _, sidecar := range l.sConfig.Sidecars {
		if sidecar.ArtifactURI == "" {
			continue
		}
		entry := entryG.WithField("sidecar", sidecar.Name)
		dir := SidecarDir(l.sConfig.Dir, sidecar.Name)
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return NewSidecarError(sidecar, err)
		}
		isEmpty, err := IsEmptyDir(dir)
		if err != nil {
			return NewSidecarError(sidecar, err)
		}
		if !isEmpty && !forceDl {
			entry.Infof("Skipping downloading from %s (directory not empty, sidecar must be already downloaded)", sidecar.ArtifactURI)
			return nil
		}
		if !isEmpty {
			err := os.RemoveAll(dir)
			if err != nil {
				return NewSidecarError(sidecar, err)
			}
			err = os.MkdirAll(dir, os.ModePerm)
			if err != nil {
				return NewSidecarError(sidecar, err)
			}
		}
		err = DownloadSidecar(dir, sidecar)
		if err != nil {
			return NewSidecarError(sidecar, err)
		}

		if sidecar.AfterDownload == "" {
			continue
		}
		if runtime.GOOS == "windows" {
			return nil
		}
		entry.Info("Run after install script ...")
		cmd := exec.Command("bash", "-c", sidecar.AfterDownload)
		cmd.Dir = filepath.Dir(l.sidecarExecPath(sidecar))
		env, err := l.overrideEnv(utils.OsEnvToMap(), sidecar.Env)
		if err != nil {
			return NewSidecarError(sidecar, err)
		}
		cmd.Env = utils.EnvMapToOsEnv(env)
		cmd.Stdout = l.stdout
		cmd.Stderr = l.stderr
		if err != nil {
			return NewSidecarError(sidecar, err)
		}
		err = cmd.Run()
		if err != nil {
			return NewSidecarError(sidecar, err)
		}
		entry.Info("Finished running after install script.")
	}
	entryG.Info("Finished downloading artifacts from sidecars.")
	return nil
}

func (l Launcher) Launch() error {
	entryG := log.WithField("component", "Launcher").
		WithField("command", "launch")
	appEnv := utils.OsEnvToMap()

	var wg sync.WaitGroup
	processLen := len(l.sConfig.Sidecars)
	if !l.sConfig.NoStarter {
		processLen++
	}
	wg.Add(processLen)

	processes := make([]*exec.Cmd, processLen)
	pProcesses := &processes

	errChan := make(chan error, 100)
	signalChan := make(chan os.Signal, 100)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)

	// manage graceful shutdown
	go func() {
		sig := <-signalChan
		// If signal has been set by other process at init we are waiting
		// to reach number of process required before sending back signal
		for !processesNotHaveLen(*pProcesses, processLen) {
			time.Sleep(10 * time.Millisecond)
		}
		for _, process := range *pProcesses {
			if process.Process == nil {
				continue // process is not running (which probably create signal)
			}
			// resent signal for each process to make them detect
			// when they receive a signal to not show error
			signalChan <- sig
			// if setpgid exist in sysproc, we need to send signal to negative pid (-pid)
			// we override pid value to let us use process.Process.Signal
			// instead of non os agnostic syscall funcs
			if utils.HasPgidSysProcAttr(process.SysProcAttr) {
				process.Process.Pid = -process.Process.Pid
			}
			process.Process.Signal(sig)
		}
		// if processes still doesn't stop after 20 sec we force shutdown
		time.Sleep(20 * time.Second)
		for _, process := range *pProcesses {
			signalChan <- syscall.SIGKILL
			process.Process.Kill()
		}
	}()
	var err error
	i := 0
	appPort := l.appPort
	if os.Getenv(AppPortEnvKey) != "" {
		appPort, err = strconv.Atoi(os.Getenv(AppPortEnvKey))
		if err != nil {
			return err
		}
	}
	for _, sidecar := range l.sConfig.Sidecars {
		env, err := l.overrideEnv(utils.OsEnvToMap(), sidecar.Env)
		if err != nil {
			return NewSidecarError(sidecar, err)
		}
		if sidecar.IsRproxy {
			if l.cStarter != nil && !l.sConfig.NoStarter {
				env, err = l.overrideEnv(env, l.cStarter.ProxyEnv(appPort))
				if err != nil {
					return NewSidecarError(sidecar, err)
				}
			}
			appPort++
			env, err = l.overrideEnv(env, map[string]string{
				ProxyAppPortEnvKey: fmt.Sprintf("%d", appPort),
			})
			if err != nil {
				return NewSidecarError(sidecar, err)
			}
		}
		entry := entryG.WithField("sidecar", sidecar.Name)
		entry.Info("Starting sidecar ...")
		appEnvUnTpl, err := l.templatingEnv(appEnv, sidecar.AppEnv)
		if err != nil {
			return NewSidecarError(sidecar, err)
		}
		appEnv = utils.MergeEnv(appEnv, appEnvUnTpl)
		cmd, err := l.cmdSidecar(sidecar, env)
		if err != nil {
			return NewSidecarError(sidecar, err)
		}
		processes[i] = cmd
		i++
		go func() {
			defer wg.Done()
			err := cmd.Run()
			if err != nil {
				select {
				case <-signalChan:
					return
				default:
				}
				errMess := fmt.Sprintf("Error occurred on sidecar %s: %s", sidecar.Name, err.Error())
				entry.Error(errMess)
				if !sidecar.NoInterruptWhenStop {
					errChan <- fmt.Errorf(errMess)
					signalChan <- syscall.SIGINT
				}
			}
		}()
		entry.Info("Sidecar has started")
	}

	if l.sConfig.NoStarter {
		wg.Wait()
		select {
		case err := <-errChan:
			return err
		default:
			return nil
		}
	}
	entryS := entryG.WithField("starter", l.cStarter.CloudEnvName())
	if appPort != l.appPort {
		appEnv = utils.MergeEnv(appEnv, l.cStarter.ProxyEnv(appPort))
	}
	entryS.Info("Running cloud starter ...")
	cloudCmd, err := l.cStarter.StartCmd(
		utils.EnvMapToOsEnv(appEnv),
		l.profileDir,
		l.stdout,
		l.stderr,
	)
	if err != nil {
		return err
	}
	processes[i] = cloudCmd

	go func() {
		defer wg.Done()
		err := cloudCmd.Run()
		if err != nil {
			select {
			case <-signalChan:
				return
			default:
			}
			errMess := fmt.Sprintf("Error occurred on cloud Launcher: %s", err.Error())
			entryS.Error(errMess)
			errChan <- fmt.Errorf(errMess)
		}
		// if main real process stopped we should stop all other processes
		signalChan <- syscall.SIGINT
	}()

	wg.Wait()
	select {
	case err = <-errChan:
		return err
	default:
		return nil
	}
	return nil
}

func (l Launcher) overrideEnv(old, new map[string]string) (map[string]string, error) {
	newUnTpl, err := l.templatingEnv(old, new)
	if err != nil {
		return map[string]string{}, err
	}
	return utils.MergeEnv(old, newUnTpl), nil
}

func (l Launcher) templatingEnv(old, new map[string]string) (map[string]string, error) {
	for k, v := range new {
		newV, err := l.templatingFromEnv(old, v)
		if err != nil {
			return new, err
		}
		new[k] = newV
	}
	return new, nil
}

func (l Launcher) templatingArgs(env map[string]string, args ...string) ([]string, error) {
	var err error
	for i, arg := range args {
		args[i], err = l.templatingFromEnv(env, arg)
		if err != nil {
			return args, err
		}
	}
	return args, nil
}

func (l Launcher) templatingFromEnv(env map[string]string, s string) (string, error) {
	// sigil allow $ENV_VAR in templating
	buf, err := sigil.Execute([]byte(s), utils.MapCast(env), "env-tpl")
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (l Launcher) sidecarExecPath(sidecar *config.SidecarConfig) string {
	execPath := sidecar.Executable
	wd := l.sConfig.Dir
	if wd == "" {
		wd, _ = os.Getwd()
	}
	if sidecar.ArtifactURI != "" {
		execPath = filepath.Join(SidecarDir(wd, sidecar.Name), execPath)
	}
	return execPath
}

func (l Launcher) cmdSidecar(sidecar *config.SidecarConfig, env map[string]string) (*exec.Cmd, error) {
	var err error
	wd := l.sConfig.Dir
	if sidecar.WorkDir != "" {
		wd = sidecar.WorkDir
	}
	if wd == "" {
		wd, _ = os.Getwd()
	}

	if _, err := os.Stat(wd); os.IsNotExist(err) {
		return nil, fmt.Errorf("Workdir '%s' doesn't exists.", wd)
	}

	args, err := l.templatingArgs(env, sidecar.Args...)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(l.sidecarExecPath(sidecar), args...)
	cmd.Env = utils.EnvMapToOsEnv(env)
	cmd.Dir = wd
	// set pgid for sending signal to child
	cmd.SysProcAttr = utils.PgidSysProcAttr()
	if !sidecar.NoLogPrefix {
		writerPrefix := fmt.Sprintf("[sidecar:%s]", sidecar.Name)
		err := PrefixCmdOutput(l.stdout, l.stderr, cmd, writerPrefix)
		if err != nil {
			return nil, err
		}
	} else {
		cmd.Stdout = l.stdout
		cmd.Stderr = l.stderr
	}

	return cmd, nil
}

func SidecarDir(baseDir, sidecarName string) string {
	return filepath.Join(baseDir, PathSidecarsWd, sidecarName)
}

func processesNotHaveLen(processes []*exec.Cmd, len int) bool {
	var i int
	var p *exec.Cmd
	for i, p = range processes {
		if p == nil {
			break
		}
	}
	return (i + 1) == len
}
