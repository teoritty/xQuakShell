// Command plugin-ui-fake exercises the ui capability (ADR-015) end to end: it opens surfaces,
// writes to them, opens dialogs and answers node-details requests, so the whole path can be tested
// against a real plugin process rather than an in-process fake.
//
// Like plugin-discovery-fake it is technology-neutral. It draws one made-up resource; naming it
// after a container would teach a reader that surfaces are a Docker feature, which is exactly what
// ADR-015 is careful not to be.
package main

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"sync"

	"xquakshell/test/fixtures/pluginhost"
)

const (
	rootNodeID = "thing"
	rootLabel  = "Fixture thing"
)

type plugin struct {
	host *pluginhost.Host

	mu        sync.Mutex
	sessionID string
	// surfaces the fixture has open, so the test can ask what it holds.
	surfaces map[string]string
	// lastDialogID is the dialog awaiting an answer, and lastAnswer records how it ended, so the
	// test can prove exactly one of submit/cancel arrived.
	lastDialogID string
	lastAnswer   string
	answerValues map[string]string
	// closedSurfaces records surface.closed notifications, which is how a plugin learns its tab is
	// gone for a reason it did not cause.
	closedSurfaces []string
	inputs         []string
	resizes        []string
}

func main() {
	p := &plugin{
		surfaces:     make(map[string]string),
		answerValues: make(map[string]string),
	}
	p.host = pluginhost.NewHost()

	p.host.Register("initialize", func(json.RawMessage) (any, error) { return map[string]bool{"ok": true}, nil })
	p.host.Register("activate", func(json.RawMessage) (any, error) { return map[string]bool{"ok": true}, nil })
	p.host.Register("ping", func(json.RawMessage) (any, error) { return map[string]string{"pong": "ok"}, nil })
	p.host.Register("shutdown", func(json.RawMessage) (any, error) { return map[string]bool{"ok": true}, nil })

	p.host.RegisterNotification("discovery.observe", p.observe)
	p.host.Register("discovery.invokeAction", p.invokeAction)
	p.host.Register("discovery.describeNode", p.describeNode)
	p.host.Register("discovery.applyDetails", p.applyDetails)

	p.host.RegisterNotification("surface.input", p.surfaceInput)
	p.host.RegisterNotification("surface.resize", p.surfaceResize)
	p.host.RegisterNotification("surface.closed", p.surfaceClosed)
	p.host.RegisterNotification("dialog.submit", p.dialogSubmit)
	p.host.RegisterNotification("dialog.cancel", p.dialogCancel)

	// The test's window into what the fixture observed. A plugin would not have this; it exists so
	// assertions can be made about the plugin's own view rather than only about the host's.
	p.host.Register("fixture.state", p.state)

	if err := p.host.Run(); err != nil {
		log.Fatal(err)
	}
}

type observeParams struct {
	SessionID string   `json:"sessionId"`
	NodeIDs   []string `json:"nodeIds"`
}

// observe publishes one instance node under the connection root, on its own goroutine: publish is
// a request, and answering a notification inline would block the read loop on a response only that
// loop can deliver.
func (p *plugin) observe(params json.RawMessage) {
	var req observeParams
	if err := json.Unmarshal(params, &req); err != nil {
		log.Printf("observe: %v", err)
		return
	}
	p.mu.Lock()
	p.sessionID = req.SessionID
	p.mu.Unlock()

	watchesRoot := false
	for _, id := range req.NodeIDs {
		if id == "" {
			watchesRoot = true
		}
	}
	if !watchesRoot {
		return
	}
	go func() {
		_, err := p.host.CallCore("discovery.publish", map[string]any{
			"sessionId": req.SessionID,
			"nodeId":    "",
			"state":     "ready",
			"children": []map[string]any{{
				"id":    rootNodeID,
				"kind":  "instance",
				"label": rootLabel,
				"actions": []map[string]any{
					{"id": "open-log", "label": "Open log"},
					{"id": "open-terminal", "label": "Open console"},
					{"id": "open-form", "label": "Open form"},
					{"id": "open-detail", "label": "Inspect"},
				},
			}},
		})
		if err != nil {
			log.Printf("publish: %v", err)
		}
	}()
}

type invokeParams struct {
	SessionID string   `json:"sessionId"`
	NodeIDs   []string `json:"nodeIds"`
	ActionID  string   `json:"actionId"`
}

// invokeAction acknowledges immediately and does the work on a goroutine, the contract every
// discovery action follows (ADR-014): the ack must arrive inside 5 s whatever the work costs.
func (p *plugin) invokeAction(params json.RawMessage) (any, error) {
	var req invokeParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	go p.runAction(req)
	return map[string]bool{"ok": true}, nil
}

func (p *plugin) runAction(req invokeParams) {
	switch req.ActionID {
	case "open-log":
		p.openSurface(req.SessionID, "log", "fixture log", "stdout line\n")
	case "open-terminal":
		p.openSurface(req.SessionID, "terminal", "fixture shell", "$ ")
	case "open-form":
		p.openDialog("form")
	case "open-detail":
		p.openDialog("detail")
	}
}

func (p *plugin) openSurface(sessionID, kind, title, first string) {
	raw, err := p.host.CallCore("surface.open", map[string]any{
		"parentSessionId": sessionID,
		"kind":            kind,
		"title":           title,
	})
	if err != nil {
		log.Printf("surface.open(%s): %v", kind, err)
		return
	}
	var res struct {
		SurfaceID string `json:"surfaceId"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		log.Printf("surface.open decode: %v", err)
		return
	}

	p.mu.Lock()
	p.surfaces[res.SurfaceID] = kind
	p.mu.Unlock()

	if _, err := p.host.CallCore("surface.write", map[string]any{
		"surfaceId":  res.SurfaceID,
		"dataBase64": base64.StdEncoding.EncodeToString([]byte(first)),
		"stream":     "stdout",
	}); err != nil {
		log.Printf("surface.write: %v", err)
	}
	if _, err := p.host.CallCore("surface.updateState", map[string]any{
		"surfaceId": res.SurfaceID,
		"state":     "ready",
	}); err != nil {
		log.Printf("surface.updateState: %v", err)
	}
}

func (p *plugin) openDialog(kind string) {
	raw, err := p.host.CallCore("dialog.open", map[string]any{
		"kind":  kind,
		"title": "Fixture dialog",
		"sections": []map[string]any{{
			"id":    "main",
			"label": "Main",
			"fields": []map[string]any{
				{"id": "name", "label": "Name", "type": "text", "secret": false},
				{"id": "labels", "label": "Labels", "type": "keyValue", "secret": false},
			},
		}},
	})
	if err != nil {
		log.Printf("dialog.open(%s): %v", kind, err)
		return
	}
	var res struct {
		DialogID string `json:"dialogId"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return
	}
	p.mu.Lock()
	p.lastDialogID = res.DialogID
	p.mu.Unlock()
}

type nodeParams struct {
	SessionID string            `json:"sessionId"`
	NodeID    string            `json:"nodeId"`
	Values    map[string]string `json:"values"`
}

func (p *plugin) describeNode(params json.RawMessage) (any, error) {
	var req nodeParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	return map[string]any{
		"editable": true,
		"values":   map[string]string{"shell": "/bin/sh"},
		"sections": []map[string]any{{
			"id":    "console",
			"label": "Console",
			"fields": []map[string]any{
				{"id": "shell", "label": "Shell", "type": "text", "secret": false},
			},
		}},
	}, nil
}

func (p *plugin) applyDetails(params json.RawMessage) (any, error) {
	var req nodeParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	p.mu.Lock()
	for k, v := range req.Values {
		p.answerValues["details."+k] = v
	}
	p.mu.Unlock()
	return map[string]bool{"ok": true}, nil
}

type surfaceNote struct {
	SurfaceID  string `json:"surfaceId"`
	DataBase64 string `json:"dataBase64"`
	Reason     string `json:"reason"`
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
}

func (p *plugin) surfaceInput(params json.RawMessage) {
	var note surfaceNote
	if err := json.Unmarshal(params, &note); err != nil {
		return
	}
	p.mu.Lock()
	p.inputs = append(p.inputs, note.SurfaceID+":"+note.DataBase64)
	p.mu.Unlock()
}

func (p *plugin) surfaceResize(params json.RawMessage) {
	var note surfaceNote
	if err := json.Unmarshal(params, &note); err != nil {
		return
	}
	p.mu.Lock()
	p.resizes = append(p.resizes, note.SurfaceID)
	p.mu.Unlock()
}

func (p *plugin) surfaceClosed(params json.RawMessage) {
	var note surfaceNote
	if err := json.Unmarshal(params, &note); err != nil {
		return
	}
	p.mu.Lock()
	p.closedSurfaces = append(p.closedSurfaces, note.SurfaceID)
	delete(p.surfaces, note.SurfaceID)
	p.mu.Unlock()
}

type dialogNote struct {
	DialogID string            `json:"dialogId"`
	Values   map[string]string `json:"values"`
}

func (p *plugin) dialogSubmit(params json.RawMessage) {
	var note dialogNote
	if err := json.Unmarshal(params, &note); err != nil {
		return
	}
	p.mu.Lock()
	p.lastAnswer = "submit"
	for k, v := range note.Values {
		p.answerValues[k] = v
	}
	p.mu.Unlock()
}

func (p *plugin) dialogCancel(params json.RawMessage) {
	var note dialogNote
	if err := json.Unmarshal(params, &note); err != nil {
		return
	}
	p.mu.Lock()
	p.lastAnswer = "cancel"
	p.mu.Unlock()
}

func (p *plugin) state(json.RawMessage) (any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	surfaces := make(map[string]string, len(p.surfaces))
	for k, v := range p.surfaces {
		surfaces[k] = v
	}
	values := make(map[string]string, len(p.answerValues))
	for k, v := range p.answerValues {
		values[k] = v
	}
	return map[string]any{
		"sessionId":      p.sessionID,
		"surfaces":       surfaces,
		"lastDialogId":   p.lastDialogID,
		"lastAnswer":     p.lastAnswer,
		"answerValues":   values,
		"closedSurfaces": append([]string(nil), p.closedSurfaces...),
		"inputs":         append([]string(nil), p.inputs...),
		"resizes":        append([]string(nil), p.resizes...),
	}, nil
}
