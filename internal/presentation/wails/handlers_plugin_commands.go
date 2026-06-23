package wails

import (
	"context"
	"encoding/json"

	"ssh-client/internal/usecase"
)

// PluginCommandDTO describes a contributed command for the command palette.
type PluginCommandDTO struct {
	PluginID string `json:"pluginId"`
	ID       string `json:"id"`
	FullID   string `json:"fullId"`
	Title    string `json:"title"`
	Category string `json:"category,omitempty"`
}

// PluginViewDTO describes a contributed sidebar/webview panel.
type PluginViewDTO struct {
	PluginID string `json:"pluginId"`
	ID       string `json:"id"`
	FullID   string `json:"fullId"`
	Location string `json:"location"`
	Title    string `json:"title"`
	Type     string `json:"type,omitempty"`
	Entry    string `json:"entry,omitempty"`
	AssetURL string `json:"assetUrl"`
}

// PluginStatusBarDTO describes a contributed status bar item.
type PluginStatusBarDTO struct {
	PluginID string `json:"pluginId"`
	ID       string `json:"id"`
	Text     string `json:"text"`
	Tooltip  string `json:"tooltip,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

// PluginContributionsDTO merges plugin UI contributions.
type PluginContributionsDTO struct {
	Commands  []PluginCommandDTO   `json:"commands"`
	Views     []PluginViewDTO      `json:"views"`
	StatusBar []PluginStatusBarDTO `json:"statusBar"`
}

// GetPluginContributions returns merged plugin contributions for the UI.
func (a *AppAPI) GetPluginContributions() PluginContributionsDTO {
	if a.plugins == nil {
		return PluginContributionsDTO{
			Commands:  []PluginCommandDTO{},
			Views:     []PluginViewDTO{},
			StatusBar: []PluginStatusBarDTO{},
		}
	}
	merged := usecase.MergeContributionsFiltered(a.plugins.Registry(), a.plugins.IsPluginEnabled)

	commands := make([]PluginCommandDTO, 0, len(merged.Commands))
	for _, cmd := range merged.Commands {
		commands = append(commands, commandToDTO(cmd))
	}

	views := make([]PluginViewDTO, 0, len(merged.Views))
	for _, view := range merged.Views {
		views = append(views, viewToDTO(view))
	}

	statusBar := make([]PluginStatusBarDTO, 0, len(merged.StatusBar))
	for _, item := range merged.StatusBar {
		statusBar = append(statusBar, statusBarToDTO(item))
	}

	return PluginContributionsDTO{
		Commands:  commands,
		Views:     views,
		StatusBar: statusBar,
	}
}

// ExecutePluginCommand runs a plugin-contributed command.
func (a *AppAPI) ExecutePluginCommand(pluginID, commandID string, args json.RawMessage) (json.RawMessage, error) {
	if a.plugins == nil {
		return nil, errPluginManagerUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginCallTimeout)
	defer cancel()
	return a.plugins.ExecuteCommand(ctx, pluginID, commandID, args)
}

func commandToDTO(cmd usecase.MergedCommand) PluginCommandDTO {
	return PluginCommandDTO{
		PluginID: cmd.PluginID,
		ID:       cmd.Command.ID,
		FullID:   cmd.FullCommandID(),
		Title:    cmd.Command.Title,
		Category: cmd.Command.Category,
	}
}

func viewToDTO(view usecase.MergedView) PluginViewDTO {
	entry := view.View.Entry
	if entry == "" {
		entry = "ui/index.html"
	}
	viewType := view.View.Type
	if viewType == "" {
		viewType = "webview"
	}
	return PluginViewDTO{
		PluginID: view.PluginID,
		ID:       view.View.ID,
		FullID:   view.FullViewID(),
		Location: view.View.Location,
		Title:    view.View.Title,
		Type:     viewType,
		Entry:    entry,
		AssetURL: view.AssetURL(),
	}
}

func statusBarToDTO(item usecase.MergedStatusBarItem) PluginStatusBarDTO {
	return PluginStatusBarDTO{
		PluginID: item.PluginID,
		ID:       item.Item.ID,
		Text:     item.Item.Text,
		Tooltip:  item.Item.Tooltip,
		Priority: item.Item.Priority,
	}
}
