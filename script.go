package sidecars

import (
	"io"
	"os/exec"
	"runtime"
)

func runScript(script, wd string, env []string, stdout, stderr io.Writer) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	cmd := exec.Command("bash", "-c", script)
	cmd.Dir = wd
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
