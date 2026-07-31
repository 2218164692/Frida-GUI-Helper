package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	runtimeenv "frida-gui-helper/internal/runtime"
	"frida-gui-helper/internal/toolchain"
)

func main() {
	output := flag.String("output", "frida", "Frida bundle output directory")
	version := flag.String("version", "latest", "Exact Frida version or latest")
	flag.Parse()

	manager := runtimeenv.NewManager(toolchain.NewResolver())
	if err := manager.SetBundleRoot(*output); err != nil {
		fatal(err)
	}
	manager.SetProgressEmitter(func(progress runtimeenv.Progress) {
		message := progress.Message
		if progress.TotalBytes > 0 {
			message = fmt.Sprintf("%s (%.0f%%)", message, progress.Percent)
		}
		if message != "" {
			fmt.Println(message)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	manifest, err := manager.InstallBundle(ctx, *version)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("Prepared Frida %s in %s\n", manifest.Version, manager.BundleRoot())
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
