package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"mimir/workflow"
)

type playbookView struct {
	workflow.Definition
	Protected bool `json:"protected"`
}

func (a *App) ensurePlaybookStore() (*workflow.PlaybookStore, error) {
	if a.PlaybookStore != nil {
		return a.PlaybookStore, nil
	}

	store, err := workflow.NewDefaultPlaybookStore()
	if err != nil {
		return nil, err
	}
	a.PlaybookStore = store
	return store, nil
}

func (a *App) GetPlaybooksJSON() (string, error) {
	store, err := a.ensurePlaybookStore()
	if err != nil {
		return "", err
	}

	playbooks, err := store.List()
	if err != nil {
		return "", fmt.Errorf("failed to load playbooks: %w", err)
	}

	items := make([]playbookView, 0, len(playbooks))
	for _, playbook := range playbooks {
		items = append(items, playbookView{
			Definition: playbook,
			Protected:  store.IsProtectedID(playbook.ID),
		})
	}

	payload, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("failed to encode playbooks: %w", err)
	}
	return string(payload), nil
}

func (a *App) SavePlaybookJSON(definitionJSON string, previousID string) (string, error) {
	store, err := a.ensurePlaybookStore()
	if err != nil {
		return "", err
	}

	var definition workflow.Definition
	if err := strictUnmarshalJSON(definitionJSON, &definition); err != nil {
		return "", fmt.Errorf("failed to parse playbook definition: %w", err)
	}

	saved, err := store.Save(definition, strings.TrimSpace(previousID))
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(playbookView{
		Definition: saved,
		Protected:  false,
	})
	if err != nil {
		return "", fmt.Errorf("failed to encode saved playbook: %w", err)
	}
	return string(payload), nil
}

func (a *App) DeletePlaybook(playbookID string) error {
	store, err := a.ensurePlaybookStore()
	if err != nil {
		return err
	}
	if strings.TrimSpace(playbookID) == "" {
		return fmt.Errorf("playbook ID is required")
	}
	return store.Delete(playbookID)
}
