package sidecars

import (
	"fmt"
	"github.com/olekukonko/tablewriter"
	"github.com/orange-cloudfoundry/cloud-sidecars/config"
	"github.com/orange-cloudfoundry/cloud-sidecars/starter"
	"github.com/orange-cloudfoundry/cloud-sidecars/utils"
	log "github.com/sirupsen/logrus"
	"gopkg.in/alessio/shellescape.v1"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

const (
	ProxyAppPortEnvKey = "PROXY_APP_PORT"
	AppPortEnvKey      = "SIDECAR_APP_PORT"
	PathSidecarsWd     = ".sidecars"
)

type Launcher struct {
	sConfig        config.Sidecars
	cStarter       starter.Starter
	profileDir     string
	stdout         io.Writer
	stderr         io.Writer
	appPort        int
	processFactory *ProcessFactory
	indexer        *Indexer
}

func NewLauncher(
	sConfig config.Sidecars,
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
		sConfig:        sConfig,
		cStarter:       cStarter,
		profileDir:     profileDir,
		stdout:         stdout,
		stderr:         stderr,
		appPort:        appPort,
		processFactory: NewProcessFactory(stdout, stderr, cStarter, sConfig.Dir),
		indexer:        NewIndexer(IndexFilePath(sConfig.Dir)),
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

func (l Launcher) setupSidecarArtifact(sidecar *config.Sidecar) error {
	entry := log.WithField("sidecar", sidecar.Name)
	entry.Debug("Unzipping artifact ...")
	index, ok := l.indexer.Index(sidecar)
	if !ok {
		return nil
	}
	zipFilePath := filepath.Join(l.sConfig.Dir, index.ZipFile)
	uz := NewUnzip(zipFilePath, filepath.Dir(zipFilePath))
	err := uz.Extract()
	if err != nil {
		return NewSidecarError(sidecar, err)
	}
	l.indexer.RemoveIndex(index)
	err = l.indexer.Store()
	if err != nil {
		return err
	}
	entry.Debug("Finished unzipping artifact ...")

	if sidecar.AfterInstall == "" {
		return nil
	}

	entry.Debug("Run after install script ...")
	env, err := OverrideEnv(utils.OsEnvToMap(), sidecar.Env)
	if err != nil {
		return NewSidecarError(sidecar, err)
	}
	err = runScript(
		sidecar.AfterInstall,
		filepath.Dir(SidecarExecPath(l.sConfig.Dir, sidecar)),
		utils.EnvMapToOsEnv(env),
		l.stdout, l.stderr,
	)
	if err != nil {
		return NewSidecarError(sidecar, err)
	}
	entry.Debug("Finished running after install script.")
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
	err = l.DownloadArtifacts()
	if err != nil {
		return err
	}
	appPort := l.appPort
	for id, sidecar := range l.sConfig.Sidecars {
		entry := entryG.WithField("sidecar", sidecar.Name)
		entry.Infof("Setup ...")

		err := l.setupSidecarArtifact(sidecar)
		if err != nil {
			return err
		}

		appEnvUnTpl, err := TemplatingEnv(appEnv, sidecar.AppEnv)
		if err != nil {
			return err
		}
		appEnv = utils.MergeEnv(appEnv, appEnvUnTpl)
		if sidecar.IsRproxy {
			appPort++
		}
		if sidecar.ProfileD != "" {
			fileName := fmt.Sprintf("%d_%s.sh", id+1, sidecar.Name)
			entry.Infof("Writing profiled file '%s' ...", fileName)
			err := os.WriteFile(
				filepath.Join(l.profileDir, fileName),
				[]byte(sidecar.ProfileD), 0755)
			if err != nil {
				return err
			}
			entry.Infof("Finished writing profiled file '%s' .", fileName)
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
	entryG.WithField("starter", l.cStarter.Name()).Info("Adding starter.sh profile")
	profileLaunch := ""
	for k, v := range appEnv {
		profileLaunch += fmt.Sprintf("export %s=%s\n", k, shellescape.Quote(v))
	}
	err = os.WriteFile(
		filepath.Join(l.profileDir, "0_starter.sh"),
		[]byte(profileLaunch), 0755)
	if err != nil {
		return err
	}
	entryG.WithField("starter", l.cStarter.Name()).Info("Finished adding starter.sh profile")
	return nil
}

func (l Launcher) DownloadArtifacts() error {
	entryG := log.WithField("component", "Launcher").WithField("command", "download_artifact")
	entryG.Info("Start downloading artifacts from sidecars ...")
	for _, sidecar := range l.sConfig.Sidecars {
		if sidecar.ArtifactURI == "" {
			continue
		}
		entry := entryG.WithField("sidecar", sidecar.Name)

		shouldDownload, why := l.indexer.ShouldDownload(sidecar)
		if !shouldDownload && why != "" {
			return NewSidecarError(sidecar, fmt.Errorf(why))
		}
		if !shouldDownload {
			entry.Info("Skipping downloading, already downloaded.")
			continue
		}
		dir := SidecarDir(l.sConfig.Dir, sidecar.Name)
		os.RemoveAll(dir)
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return NewSidecarError(sidecar, err)
		}
		zipFileName := sidecar.Name + ".zip"
		zipFilePath := filepath.Join(dir, zipFileName)
		err = DownloadSidecar(zipFilePath, sidecar)
		if err != nil {
			return NewSidecarError(sidecar, err)
		}

		err = l.indexer.UpdateOrCreateIndex(sidecar, filepath.Join(PathSidecarsWd, sidecar.Name, zipFileName))
		if err != nil {
			os.Remove(zipFilePath)
			return NewSidecarError(sidecar, err)
		}

		err = l.indexer.Store()
		if err != nil {
			return err
		}
	}
	log.Debug("Cleaning non existing sidecars ...")
	indexToRm := l.indexer.IndexToRemove(l.sConfig.Sidecars)
	for _, index := range indexToRm {
		os.RemoveAll(filepath.Dir(index.ZipFile))
		l.indexer.RemoveIndex(index)
		l.indexer.Store()
	}
	log.Debug("Finished cleaning non existing sidecars ...")

	entryG.Info("Finished downloading artifacts from sidecars.")
	return nil
}

func (l Launcher) Launch() error {
	entry := log.WithField("component", "Launcher").
		WithField("command", "launch")

	wg := l.processFactory.WaitGroup()
	processLen := len(l.sConfig.Sidecars)
	if !l.sConfig.NoStarter {
		processLen++
	}
	entry.Info("Creating all processes ...")
	processLen, processes, err := l.CreateProcesses()
	if err != nil {
		return err
	}
	entry.Info("Finished creating all processes ...")

	wg.Add(processLen)
	pProcesses := &processes

	signalChan := l.processFactory.SignalChan()
	errChan := l.processFactory.ErrorChan()
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)

	// manage graceful shutdown
	go l.handlingSignal(pProcesses, processLen, signalChan)

	for _, p := range processes {
		go p.Start()
	}
	wg.Wait()
	select {
	case err = <-errChan:
		return err
	default:
		return nil
	}
}

func (l Launcher) CreateProcesses() (processLen int, processes []*process, err error) {
	processLen = len(l.sConfig.Sidecars)
	if !l.sConfig.NoStarter {
		processLen++
	}
	processes = make([]*process, processLen)

	appEnv := utils.OsEnvToMap()
	i := 0
	appPort := l.appPort
	if os.Getenv(AppPortEnvKey) != "" {
		appPort, err = strconv.Atoi(os.Getenv(AppPortEnvKey))
		if err != nil {
			return processLen, processes, err
		}
	}
	for _, sidecar := range l.sConfig.Sidecars {
		env, err := OverrideEnv(utils.OsEnvToMap(), sidecar.Env)
		if err != nil {
			return processLen, processes, NewSidecarError(sidecar, err)
		}
		if sidecar.IsRproxy {
			if l.cStarter != nil && !l.sConfig.NoStarter {
				env, err = OverrideEnv(env, l.cStarter.ProxyEnv(appPort))
				if err != nil {
					return processLen, processes, NewSidecarError(sidecar, err)
				}
			}
			appPort++
			env, err = OverrideEnv(env, map[string]string{
				ProxyAppPortEnvKey: fmt.Sprintf("%d", appPort),
			})
			if err != nil {
				return processLen, processes, NewSidecarError(sidecar, err)
			}
		}
		entry := log.WithField("sidecar", sidecar.Name)
		entry.Debug("Setup sidecar ...")
		appEnvUnTpl, err := TemplatingEnv(appEnv, sidecar.AppEnv)
		if err != nil {
			return processLen, processes, NewSidecarError(sidecar, err)
		}
		appEnv = utils.MergeEnv(appEnv, appEnvUnTpl)
		processes[i], err = l.processFactory.FromSidecar(sidecar, env)
		if err != nil {
			return processLen, processes, NewSidecarError(sidecar, err)
		}
		i++

		entry.Debug("Finished setup sidecar.")
	}
	if !l.sConfig.NoStarter {
		entryS := log.WithField("starter", l.cStarter.Name())
		if appPort != l.appPort {
			appEnv = utils.MergeEnv(appEnv, l.cStarter.ProxyEnv(appPort))
		}
		entryS.Debug("Setup cloud starter ...")
		processes[i], err = l.processFactory.FromStarter(appEnv, l.profileDir)
		if err != nil {
			return processLen, processes, err
		}
		entryS.Debug("Finished setup cloud starter ...")
	}
	return processLen, processes, err
}

func (l Launcher) handlingSignal(pProcesses *[]*process, processLen int, signalChan chan os.Signal) {
	sig := <-signalChan
	// If signal has been set by other process at init we are waiting
	// to reach number of process required before sending back signal
	for !processesNotHaveLen(*pProcesses, processLen) {
		time.Sleep(10 * time.Millisecond)
	}
	for _, process := range *pProcesses {
		if process.cmd.Process == nil {
			continue // process is not running (which probably create signal)
		}
		// resent signal for each process to make them detect
		// when they receive a signal to not show error
		signalChan <- sig
		// if setpgid exist in sysproc, we need to send signal to negative pid (-pid)
		// this will stop all sub process that one of our sidecars or app has started
		// we override pid value to let us use process.Process.Signal
		// instead of non os agnostic syscall funcs
		if utils.HasPgidSysProcAttr(process.cmd.SysProcAttr) {
			process.cmd.Process.Pid = -process.cmd.Process.Pid
		}
		process.cmd.Process.Signal(sig)
	}
	// if processes still doesn't stop after 20 sec we force shutdown
	time.Sleep(20 * time.Second)
	for _, process := range *pProcesses {
		signalChan <- syscall.SIGKILL
		process.cmd.Process.Kill()
	}
}

func SidecarDir(baseDir, sidecarName string) string {
	return filepath.Join(baseDir, PathSidecarsWd, sidecarName)
}

func IndexFilePath(baseDir string) string {
	return filepath.Join(baseDir, PathSidecarsWd, "index.yml")
}

func processesNotHaveLen(processes []*process, len int) bool {
	var i int
	var p *process
	for i, p = range processes {
		if p == nil {
			break
		}
	}
	return (i + 1) == len
}
