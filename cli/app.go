package main

import (
	"fmt"
	"github.com/cloudfoundry-community/gautocloud"
	"github.com/cloudfoundry-community/gautocloud/cloudenv"
	"github.com/cloudfoundry-community/gautocloud/connectors/generic"
	"github.com/cloudfoundry-community/gautocloud/interceptor/cli/urfave"
	"github.com/cloudfoundry-community/gautocloud/interceptor/configfile"
	"github.com/cloudfoundry-community/gautocloud/loader"
	"github.com/orange-cloudfoundry/cloud-sidecars"
	"github.com/orange-cloudfoundry/cloud-sidecars/config"
	"github.com/orange-cloudfoundry/cloud-sidecars/starter"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

var cliInterceptor *urfave.CliInterceptor
var confFileIntercept *configfile.ConfigFileInterceptor

const configFileName = "sidecars-config.yml"

func init() {
	os.Setenv(cloudenv.LOCAL_CONFIG_ENV_KEY, configFileName)
	confFileIntercept = configfile.NewConfigFile()
	cliInterceptor = urfave.NewCli()
	gautocloud.RegisterConnector(generic.NewConfigGenericConnector(
		config.SidecarsConfig{},
		confFileIntercept,
		cliInterceptor,
	))
}

type CloudSidecarApp struct {
	*cli.App
}

func NewApp(version string) *CloudSidecarApp {
	app := &CloudSidecarApp{cli.NewApp()}
	app.Name = "cloud-sidecar"
	app.Version = version
	app.Usage = "Cloud sidecar cli"
	app.ErrWriter = os.Stderr
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "config-path, c",
			Value:  "sidecars-config.yml",
			Usage:  "Path to the config file (This file will not be used in a cloud env like Cloud Foundry, Heroku or kubernetes)",
			EnvVar: cloudenv.LOCAL_CONFIG_ENV_KEY,
		},
		cli.StringFlag{
			Name:  "dir, d",
			Value: "",
			Usage: "Set directory where to perform commands",
		},
		cli.StringFlag{
			Name:  "log-level, l",
			Usage: "Log level to use",
		},
		cli.StringFlag{
			Name:  "cloud-env",
			Usage: "Force cloud env detection",
		},
		cli.BoolFlag{
			Name:  "log-json, j",
			Usage: "Write log in json",
		},
		cli.BoolFlag{
			Name:  "no-color",
			Usage: "Logger will not display colors",
		},
		cli.StringFlag{
			Name:  "profile-dir",
			Usage: "Set path where to put profiled files",
			Value: "",
		},
	}
	app.Commands = []cli.Command{
		{
			Name:   "launch",
			Usage:  "launch all sidecar and main process, must be run as start command",
			Action: launchRun,
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "no-starter",
					Usage: "Main process will not be started",
				},
			},
		},
		{
			Name:   "vendor",
			Usage:  "Vendor all sidecars in local for offline app",
			Action: vendorRun,
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "force, f",
					Usage: "Force downloading even if files are found for sidecar",
				},
			},
		},
		{
			Name:   "setup",
			Usage:  "Download sidecars if needed and create profiled files, this should be run by a staging lifecycle (e.g.: cloud foundry buildpack lifecycle)",
			Action: setupRun,
		},
	}
	return app
}

func setupRun(c *cli.Context) error {
	initApp(c)
	l, err := createLauncher(c, false)
	if err != nil {
		return err
	}
	return l.Setup()
}

func launchRun(c *cli.Context) error {
	initApp(c)
	l, err := createLauncher(c, true)
	if err != nil {
		return err
	}
	return l.Launch()
}

func vendorRun(c *cli.Context) error {
	initApp(c)
	l, err := createLauncher(c, false)
	if err != nil {
		return err
	}
	return l.DownloadArtifacts(c.Bool("force"))
}

func initApp(c *cli.Context) {
	loadLogConfig(&config.SidecarsConfig{
		LogJson:  c.GlobalBool("log-json"),
		LogLevel: c.GlobalString("log-level"),
		NoColor:  c.GlobalBool("no-color"),
	})
}

func createLauncher(c *cli.Context, failWhenNoStarter bool) (*sidecars.Launcher, error) {
	entry := log.WithField("component", "cli")
	entry.Debug("Creating launcher ...")
	conf, err := retrieveConfig(c)
	if err != nil {
		return nil, err
	}
	loadLogConfig(conf)

	baseDir := c.GlobalString("dir")

	profileDir := c.GlobalString("profile-dir")
	if profileDir == "" {
		profileDir = filepath.Join(baseDir, "profile.d")
	}
	var cStarter starter.Starter

	if !c.Bool("no-starter") {
		entry.Debug("Loading starter ...")
		sidecarEnv := gautocloud.CurrentCloudEnv().Name()
		if c.GlobalString("cloud-env") != "" {
			sidecarEnv = c.GlobalString("cloud-env")
		}
		for _, s := range starter.Retrieve() {
			if s.CloudEnvName() == sidecarEnv {
				log.Infof("Starter for %s is loading", s.CloudEnvName())
				cStarter = s
			}
		}
		if cStarter == nil && failWhenNoStarter {
			return nil, fmt.Errorf("Could not found starter for ")
		}
		entry.Debug("Finished loading starter.")
	}

	l := sidecars.NewLauncher(*conf, cStarter, baseDir, profileDir, os.Stdout, os.Stderr)
	entry.Debug("Finished creating launcher.")
	return l, nil
}

func retrieveConfig(c *cli.Context) (*config.SidecarsConfig, error) {
	// Has been modified in init, reset it after loading config for possible env var usage in sidecars
	defer os.Unsetenv(cloudenv.LOCAL_CONFIG_ENV_KEY)

	log.WithField("component", "cli").Debug("Loading configuration ...")
	cliInterceptor.SetContext(c)
	confPath := c.GlobalString("config-path")
	if _, err := os.Stat(confPath); os.IsNotExist(err) {
		confPath = filepath.Join(sidecars.PathSidecarsWd, configFileName)
	}
	confFileIntercept.SetConfigPath(confPath)

	conf := &config.SidecarsConfig{}
	err := gautocloud.Inject(conf)
	if _, ok := err.(loader.ErrGiveService); ok {
		log.Warnf("Cannot found configuration from gautocloud, fallback to %s file", confPath)
		var b []byte
		b, err = ioutil.ReadFile(confPath)
		if err != nil {
			return nil, fmt.Errorf("configuration error, see previous message: %s", err.Error())
		}
		err = yaml.Unmarshal(b, conf)
		if err != nil {
			return nil, fmt.Errorf("configuration error, see previous message: %s", err.Error())
		}
	}
	log.WithField("component", "cli").Debug("Finished loading configuration.")
	return conf, err
}

func loadLogConfig(c *config.SidecarsConfig) {
	if c.LogJson {
		log.SetFormatter(&log.JSONFormatter{})
	} else {
		log.SetFormatter(&log.TextFormatter{
			DisableColors: c.NoColor,
		})
	}

	if c.LogLevel == "" {
		return
	}
	switch strings.ToUpper(c.LogLevel) {
	case "ERROR":
		log.SetLevel(log.ErrorLevel)
		return
	case "WARN":
		log.SetLevel(log.WarnLevel)
		return
	case "DEBUG":
		log.SetLevel(log.DebugLevel)
		return
	case "PANIC":
		log.SetLevel(log.PanicLevel)
		return
	case "FATAL":
		log.SetLevel(log.FatalLevel)
		return
	}
}
