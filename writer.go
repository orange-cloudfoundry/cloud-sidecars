package sidecars

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
)

type CmdWriter struct {
	// nolint:unused
	cmd *exec.Cmd
}

func PrefixCmdOutput(stdout, stderr io.Writer, cmd *exec.Cmd, prefix string) error {
	stdoutCmd, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderrCmd, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	// Created scanners for in, out, and err pipes
	outScanner := bufio.NewScanner(stdoutCmd)
	errScanner := bufio.NewScanner(stderrCmd)

	// Scan for text
	go func() {
		for errScanner.Scan() {
			scannerOutput(stderr, prefix, errScanner.Text())
		}
	}()

	go func() {
		for outScanner.Scan() {
			scannerOutput(stdout, prefix, outScanner.Text())
		}
	}()

	return nil
}

func scannerOutput(writer io.Writer, prefix string, text string) {
	out := fmt.Sprintf("%s %s\n", prefix, text)
	fmt.Fprint(writer, out)
}
