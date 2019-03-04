package sidecars

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	PathSidecarsWd = ".sidecars"
)

func MergeEnv(old, new map[string]string) map[string]string {
	for k, v := range new {
		old[k] = v
	}
	return old
}

func OsEnvToMap() map[string]string {
	envv := os.Environ()
	env := make(map[string]string)
	for _, e := range envv {
		kv := strings.SplitN(e, "=", 2)
		env[kv[0]] = kv[1]
	}
	return env
}

func EnvMapToOsEnv(env map[string]string) []string {
	envv := make([]string, len(env))
	i := 0
	for k, v := range env {
		envv[i] = k + "=" + v
		i++
	}
	return envv
}

func MapCast(m map[string]string) map[string]interface{} {
	mI := make(map[string]interface{})
	for k, v := range m {
		mI[k] = v
	}
	return mI
}

func SidecarDir(baseDir, sidecarName string) string {
	return filepath.Join(baseDir, PathSidecarsWd, sidecarName)
}
