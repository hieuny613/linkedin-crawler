package utils

import (
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

// SetupSignalHandling sets up signal handling for graceful shutdown
func SetupSignalHandling(shutdownRequested *int32, onShutdown func(), sleepDuration time.Duration) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		fmt.Printf("\n⚠️ Nhận signal %v, đang shutdown...\n", sig)
		atomic.StoreInt32(shutdownRequested, 1)

		if onShutdown != nil {
			onShutdown()
		}

		fmt.Printf("💤 Sleep %v trước khi thoát...\n", sleepDuration)
		time.Sleep(sleepDuration)
		os.Exit(0)
	}()
}
