package main

import (
	"os"

	"mimir/terminal"
)

// Shell preferences exposed to Settings. They are read by the launch
// paths of the terminal package; changes apply to terminals started from
// now on (the input-method one at the next start of Mimir).

func (a *App) GetPromptMode() string { return terminal.PromptMode() }

func (a *App) SetPromptMode(mode string) (string, error) {
	mode = terminal.NormalizePromptMode(mode)
	return mode, terminal.WritePref(terminal.PromptModeFile, mode)
}

func (a *App) GetIMModule() string { return terminal.IMModule() }

func (a *App) SetIMModule(mode string) (string, error) {
	mode = terminal.NormalizeIMModule(mode)
	return mode, terminal.WritePref(terminal.IMModuleFile, mode)
}

// IMModuleEffective reports whether the simple context would be forced at
// the next start with the persisted setting and the current environment.
func (a *App) IMModuleEffective() bool {
	return terminal.ShouldForceSimpleIM(terminal.IMModule(), terminal.CurrentLocale(), envHas("GTK_IM_MODULE"))
}

func envHas(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}
