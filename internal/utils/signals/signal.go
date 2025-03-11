package signals

import (
	"os"
)

var onlyOneSignalHandler = make(chan struct{})

// SetupSignalHandler Register SIGINT and SIGTERM signals to the processor.
// When these signals are captured, turn off the STOP signal and wait for processing.
// If the signal continues to be received, exit with an exit code of 1
func SetupSignalHandler() <-chan struct{} {
	close(onlyOneSignalHandler) // panics when called twice

	stop := make(chan struct{})
	c := make(chan os.Signal, 2)

	go func() {
		<-c
		close(stop)
		<-c
		os.Exit(1)
	}()
	return stop

}
