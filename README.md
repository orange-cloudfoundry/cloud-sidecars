# Cloud-sidecars

A CLI tool to run sidecars inside an app on different cloud environment.
Sidecar will be run locally inside your app and will run real app with configuration set by sidecar.

If sidecar is a reverse proxy, it will overwrite real app configuration to make run your reverse proxy in front of the app.

For now it only support 2 types:
- Cloud foundry through its [buildpack](https://github.com/orange-cloudfoundry/cloud-sidecars-buildpack)
- Locally only for testing purpose before run

## installation

### On cloud foundry

Use buildpack associated: https://github.com/orange-cloudfoundry/cloud-sidecars-buildpack

You can also install locally to be able to run `cloud-sidecar vendor` to vendor all sidecars in local for offline app.

### Locally

#### On *nix system

You can install this via the command-line with either `curl` or `wget`.

##### via curl

```bash
$ bash -c "$(curl -fsSL https://raw.github.com/orange-cloudfoundry/cloud-sidecars/master/bin/install.sh)"
```

##### via wget

```bash
$ bash -c "$(wget https://raw.github.com/orange-cloudfoundry/cloud-sidecars/master/bin/install.sh -O -)"
```

#### On windows

You can install it by downloading the `.exe` corresponding to your cpu from releases page: https://github.com/cloud-sidecars/terraform-secure-backend/releases .
Alternatively, if you have a terminal interpreting shell you can also use command line script above, it will download file in your current working dir.

## Commands

```
NAME:
   cloud-sidecar - Cloud sidecar cli

USAGE:
   cloud-sidecars [global options] command [command options] [arguments...]

VERSION:
   dev

COMMANDS:
     launch   launch all sidecar and main process, must be run as start command
     vendor   Vendor all sidecars in local for offline app
     setup    Download sidecars if needed and create profiled files, this should be run by a staging lifecycle (e.g.: cloud foundry buildpack lifecycle)
     help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --config-path value, -c value  Path to the config file (This file will not be used in a cloud env like Cloud Foundry, Heroku or kubernetes) (default: "sidecars-config.yml") [$CONFIG_FILE]
   --dir value, -d value          Set directory where to perform commands
   --log-level value, -l value    Log level to use
   --cloud-env value              Force cloud env detection
   --log-json, -j                 Write log in json
   --no-color                     Logger will not display colors
   --profile-dir value            Set path where to put profiled files
   --help, -h                     show help
   --version, -v                  print the version
```

## Usage

By default configuration can be write as a file named `sidecars-config.yml` 
but it use [gautocloud](https://github.com/cloudfoundry-community/gautocloud) for loading configuration.
You could use instead a cups service named `sidecar-config` for cloud foundry or `SIDECAR_CONFIG_<PARAM>` for heroku/k8s.

Here the configuration file in `sidecars-config.yml` with exemple for [gobis-server]():

```yaml
sidecars:
  # Name must be defined for your sidecar
- name: gobis-server
  # Path to execute your sidecar (You can run binary set in PATH)
  # If artifact_url is set, executable path is prefixed directly with download path by cloud-sidecars
  executable: gobis-server
  # This can be empty, it let you download an artifact. Artifacts are unzipped and placed at <dir>/.sidecars/<sidecar name>
  # executable path is prefixed directly with this path by cloud-sidecars
  # work dir for after_download is this directory: <dir>/.sidecars/<sidecar name>
  # It uses https://github.com/ArthurHlt/zipper for downloading artifacts this let you download git, zip, tar, tgz or any other file (they all be uncompressed)
  artifact_url: https://github.com/orange-cloudfoundry/gobis-server/releases/download/v1.6.1/gobis-server_linux_amd64.zip
  # force type detection for https://github.com/ArthurHlt/zipper
  artifact_type: zip
  # Run script after downloading your artifact
  # here it renames gobis-server_linux_amd64 to gobis-server
  after_download: "mv * gobis-server"
  # pass args to executable
  args: 
  - "--sidecar"
  - "--sidecar-app-port"
  # this sidecar is defines as reverse proxy, it give a PROXY_APP_PORT env var
  # as bellow you can give args in posix style from env var
  - "${PROXY_APP_PORT}"
  # Set env var for sidecar
  # you can give a value in posix style from env var
  env: {}
  # Set env var for app, all app_env found in sidecars will be merged in one
  # you can give a value in posix style from env var
  app_env: {}
  # You can pass a profile file which will be source before executing app
  profiled: ""
  # Set working directory, by defaul it is the dir defined by cli flag --dir
  work_dir: ""
  # Do not put prefix in stdout/stderr for this sidecar
  no_log_prefix: false
  # If true this will override listen port for app and set an PROXY_APP_PORT env var for sidecar
  # If you have multiple sidecar of type reverse proxy it will chain in the order set here.
  is_rproxy: true
  # If true when your sidecar stop it will not stop main app and others sidecars
  no_interrupt_when_stop: false
```