package sidecars

import (
	"fmt"
	"github.com/gliderlabs/sigil"
	"github.com/orange-cloudfoundry/cloud-sidecars/config"
	"github.com/orange-cloudfoundry/cloud-sidecars/starter"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
)

type Launcher struct {
	sConfig    config.SidecarsConfig
	cStarter   starter.Starter
	baseDir    string
	profileDir string
	stdout     io.Writer
	stderr     io.Writer
	appPort    int
}

func NewLauncher(
	sConfig config.SidecarsConfig,
	cStarter starter.Starter,
	baseDir string,
	profileDir string,
	stdout, stderr io.Writer,
) *Launcher {
	appPort, _ := strconv.Atoi(os.Getenv("PORT"))
	if cStarter != nil && !sConfig.NoStarter {
		appPort = cStarter.AppPort()
	}
	return &Launcher{
		sConfig:    sConfig,
		cStarter:   cStarter,
		baseDir:    baseDir,
		profileDir: profileDir,
		stdout:     stdout,
		stderr:     stderr,
		appPort:    appPort,
	}
}

func (l Launcher) Setup() error {
	entryG := log.WithField("component", "Launcher").WithField("command", "staging")
	entryG.Infof("Setup sidecars ...")

	err := os.MkdirAll(l.profileDir, 0755)
	if err != nil {
		return err
	}
	appPort := l.appPort
	for _, sidecar := range l.sConfig.Sidecars {
		if sidecar.IsRproxy {
			appPort++
		}
		entry := entryG.WithField("sidecar", sidecar.Name)
		if sidecar.ArtifactURL == "" {
			continue
		}
		entry.Infof("Setup ...")
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
	return nil
}

func (l Launcher) DownloadArtifacts(forceDl bool) error {
	entryG := log.WithField("component", "Launcher").WithField("command", "download_artifact")
	entryG.Info("Start downloading artifacts from sidecars ...")
	for _, sidecar := range l.sConfig.Sidecars {
		if sidecar.ArtifactURL == "" {
			continue
		}
		dir := SidecarDir(l.baseDir, sidecar.Name)
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return err
		}
		err = DownloadSidecar(dir, sidecar, forceDl)
		if err != nil {
			return err
		}

		if sidecar.AfterDownload == "" {
			continue
		}

		entry := entryG.WithField("sidecar", sidecar.Name)
		entry.Info("Run after install script ...")
		cmd := exec.Command("bash", "-c", sidecar.AfterDownload)
		cmd.Dir = filepath.Dir(l.sidecarExecPath(sidecar))
		env, err := l.overrideEnv(OsEnvToMap(), sidecar.Env)
		if err != nil {
			return err
		}
		cmd.Env = EnvMapToOsEnv(env)
		cmd.Stdout = l.stdout
		cmd.Stderr = l.stderr
		if err != nil {
			return err
		}
		err = cmd.Run()
		if err != nil {
			return err
		}
		entry.Info("Finished running after install script.")
	}
	entryG.Info("Finished downloading artifacts from sidecars.")
	return nil
}

func (l Launcher) Launch() error {
	entryG := log.WithField("component", "Launcher").
		WithField("command", "launch")
	appEnv := OsEnvToMap()

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
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// manage graceful shutdown
	go func() {
		sig := <-signalChan
		for _, process := range *pProcesses {
			// resent signal for each process to make them detect when they receive a signal to not show error
			signalChan <- sig
			process.Process.Signal(sig)
		}
		// if processes still doesn't stop after 20 sec we force shutdown
		time.Sleep(20 * time.Second)
		for _, process := range *pProcesses {
			signalChan <- syscall.SIGKILL
			process.Process.Kill()
		}
	}()

	i := 0
	appPort := l.appPort
	for _, sidecar := range l.sConfig.Sidecars {
		env, err := l.overrideEnv(OsEnvToMap(), sidecar.Env)
		if err != nil {
			return err
		}
		if sidecar.IsRproxy {
			if l.cStarter != nil && !l.sConfig.NoStarter {
				env, err = l.overrideEnv(env, l.cStarter.ProxyEnv(appPort))
				if err != nil {
					return err
				}
			}
			appPort++
			env, err = l.overrideEnv(env, map[string]string{
				ProxyAppPortEnvKey: fmt.Sprintf("%d", appPort),
			})
			if err != nil {
				return err
			}
		}
		entry := entryG.WithField("sidecar", sidecar.Name)
		entry.Info("Starting sidecar ...")
		appEnvUnTpl, err := l.templatingEnv(appEnv, sidecar.AppEnv)
		if err != nil {
			return err
		}
		appEnv = MergeEnv(appEnv, appEnvUnTpl)
		cmd, err := l.cmdSidecar(sidecar, env)
		if err != nil {
			return err
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
	appEnv = MergeEnv(appEnv, l.cStarter.ProxyEnv(appPort))
	entryS.Info("Running cloud starter ...")
	cloudCmd, err := l.cStarter.StartCmd(
		EnvMapToOsEnv(appEnv),
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
	return MergeEnv(old, newUnTpl), nil
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
	buf, err := sigil.Execute([]byte(s), MapCast(env), "env-tpl")
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (l Launcher) sidecarExecPath(sidecar *config.SidecarConfig) string {
	execPath := sidecar.Executable
	if sidecar.ArtifactURL != "" {
		execPath = filepath.Join(SidecarDir(l.baseDir, sidecar.Name), execPath)
	}
	return execPath
}

func (l Launcher) cmdSidecar(sidecar *config.SidecarConfig, env map[string]string) (*exec.Cmd, error) {
	var err error
	wd := l.baseDir
	if sidecar.WorkDir != "" {
		wd = sidecar.WorkDir
	}
	if wd == "" {
		wd, _ = os.Getwd()
	}
	args, err := l.templatingArgs(env, sidecar.Args...)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(l.sidecarExecPath(sidecar), args...)
	cmd.Env = EnvMapToOsEnv(env)
	cmd.Dir = wd
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
