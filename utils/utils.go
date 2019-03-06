package utils

import (
	"os"
	"reflect"
	"strings"
	"syscall"
)

func MergeEnv(old, new map[string]string) map[string]string {
	for k, v := range new {
		old[k] = v
	}
	return old
}

func EnvToMap(envv []string) map[string]string {
	env := make(map[string]string)
	for _, e := range envv {
		kv := strings.SplitN(e, "=", 2)
		env[kv[0]] = kv[1]
	}
	return env
}

func OsEnvToMap() map[string]string {
	return EnvToMap(os.Environ())
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

func PgidSysProcAttr() *syscall.SysProcAttr {
	attr := &syscall.SysProcAttr{}
	val := reflect.ValueOf(attr).Elem()
	valSetpgid := val.FieldByName("Setpgid")
	if valSetpgid == (reflect.Value{}) || valSetpgid.Kind() != reflect.Bool {
		return nil
	}
	valSetpgid.SetBool(true)
	return attr
}

func HasPgidSysProcAttr(attr *syscall.SysProcAttr) bool {
	if attr == nil {
		return false
	}
	val := reflect.ValueOf(attr).Elem()
	valSetpgid := val.FieldByName("Setpgid")
	return valSetpgid != (reflect.Value{}) && valSetpgid.Kind() == reflect.Bool && valSetpgid.Bool()
}
