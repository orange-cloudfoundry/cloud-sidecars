package sidecars

type CmdHandler interface {
	Run() error
	Start() error
	Wait() error
}
