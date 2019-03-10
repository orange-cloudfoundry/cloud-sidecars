package sidecars

import (
	"github.com/gliderlabs/sigil"
	"github.com/orange-cloudfoundry/cloud-sidecars/utils"
)

func OverrideEnv(old, new map[string]string) (map[string]string, error) {
	newUnTpl, err := TemplatingEnv(old, new)
	if err != nil {
		return map[string]string{}, err
	}
	return utils.MergeEnv(old, newUnTpl), nil
}

func TemplatingEnv(old, new map[string]string) (map[string]string, error) {
	for k, v := range new {
		newV, err := TemplatingFromEnv(old, v)
		if err != nil {
			return new, err
		}
		new[k] = newV
	}
	return new, nil
}

func TemplatingArgs(env map[string]string, args ...string) ([]string, error) {
	var err error
	for i, arg := range args {
		args[i], err = TemplatingFromEnv(env, arg)
		if err != nil {
			return args, err
		}
	}
	return args, nil
}

func TemplatingFromEnv(env map[string]string, s string) (string, error) {
	// sigil allow $ENV_VAR in templating
	buf, err := sigil.Execute([]byte(s), utils.MapCast(env), "env-tpl")
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
