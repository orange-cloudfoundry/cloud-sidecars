package sidecars

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

type process struct {
	cmd             *exec.Cmd
	name            string
	typeP           string
	noInterrupt     bool
	alwaysInterrupt bool
	errChan         chan error
	signalChan      chan os.Signal
	wg              *sync.WaitGroup
}

func (p *process) Start() {
	entry := log.WithField(p.typeP, p.name)
	entry.Infof("Starting %s %s ...", p.typeP, p.name)
	defer p.wg.Done()
	err := p.cmd.Run()
	if err != nil {
		select {
		case <-p.signalChan:
			return
		default:
		}
		errMess := fmt.Sprintf("Error occurred on %s %s: %s", p.typeP, p.name, err.Error())
		entry.Error(errMess)
		if !p.noInterrupt {
			p.errChan <- fmt.Errorf(errMess)
			p.signalChan <- syscall.SIGINT
		}
	}
	// if process stopped we should stop all other processes
	if p.alwaysInterrupt {
		p.signalChan <- syscall.SIGINT
	}
}
