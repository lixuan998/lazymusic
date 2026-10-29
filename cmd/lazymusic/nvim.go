package main

import (
	"flag"
	"fmt"
	"lazymusic/internal/nvimconfig"
)

func setupNeovim(args []string) error {
	f := flag.NewFlagSet("setup-nvim", flag.ContinueOnError)
	configDir := f.String("config-dir", "", "Neovim configuration directory")
	pluginDir := f.String("plugin-dir", "", "installed Neovim plugin directory (without DESTDIR)")
	binary := f.String("binary", "", "installed lazymusic path (without DESTDIR)")
	helpDir := f.String("help-dir", "", "deployed Neovim help directory")
	if err := f.Parse(args); err != nil {
		return err
	}
	if len(f.Args()) > 0 {
		return fmt.Errorf("unexpected setup-nvim arguments")
	}
	if *helpDir != "" {
		if err := nvimconfig.WriteHelpTags(*helpDir); err != nil {
			return err
		}
	}
	result, err := nvimconfig.Write(*configDir, *pluginDir, *binary)
	if err != nil {
		return err
	}
	if result.Backup != "" {
		fmt.Println("Neovim config backup:", result.Backup)
	}
	status := "unchanged"
	if result.Changed {
		status = "updated"
	}
	fmt.Printf("Neovim config %s: %s\n", status, result.Path)
	return nil
}
