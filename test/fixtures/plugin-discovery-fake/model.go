package main

import "strconv"

// The wire types are declared here rather than imported from internal/domain/discovery on purpose:
// a real discovery plugin is a separate program that knows only the JSON contract of ADR-014. If
// this fixture reused the core's structs, a rename in the core would silently follow it into the
// fixture and the end-to-end test would keep passing while every third-party plugin broke.

// node is one entry of a discovery subtree, as discovery.publish carries it.
type node struct {
	ID              string   `json:"id"`
	ParentID        string   `json:"parentId"`
	Kind            string   `json:"kind"`
	Label           string   `json:"label"`
	IconID          string   `json:"iconId,omitempty"`
	Order           int      `json:"order,omitempty"`
	Status          *status  `json:"status,omitempty"`
	Actions         []action `json:"actions,omitempty"`
	DefaultActionID string   `json:"defaultActionId,omitempty"`
}

// status is the plugin's opinion about a node. Its absence is a distinct signal from a present but
// neutral tone: "nothing to report" must render differently from "reported as unremarkable".
type status struct {
	Tone    string `json:"tone"`
	Color   string `json:"color,omitempty"`
	Tooltip string `json:"tooltip,omitempty"`
}

// action is an operation the core relays without understanding it.
type action struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Danger  bool   `json:"danger,omitempty"`
	Confirm string `json:"confirm,omitempty"`
	Multi   bool   `json:"multi,omitempty"`
}

// toneNone is this fixture's marker for "the plugin reported no status at all". It never reaches
// the wire — a node carrying it is published with Status nil.
const toneNone = ""

const (
	actionInspect = "inspect"
	actionRefresh = "refresh"
	actionRetire  = "retire"
	actionRescan  = "rescan"
)

// instanceActions is the ADR-014 action matrix in miniature: one single-node action, one mass
// action, and one that is both destructive and confirmable.
var instanceActions = []action{
	{ID: actionInspect, Label: "Inspect"},
	{ID: actionRefresh, Label: "Refresh", Multi: true},
	{ID: actionRetire, Label: "Retire", Danger: true, Confirm: "Retire the selected items?", Multi: true},
}

// groupActions shows that a group carries its own actions rather than the core expanding an action
// over its children (ADR-014 "Actions").
var groupActions = []action{
	{ID: actionRescan, Label: "Rescan"},
}

// branch is one publishable branch: a parent node ID and the children it has.
//
// The slice is ordered root-first, and publishing follows that order. A snapshot for a parent the
// host has never seen is dropped, so a plugin that published "alpha" before the group containing it
// would lose the branch — level-triggered redelivery would repair it eventually, but "eventually"
// is not a thing a fixture should demonstrate.
type branch struct {
	parent   string
	children []node
}

// layout is the whole fake tree. Names are deliberately meaningless: the core is technology-neutral
// and this fixture exists to show that, not to impersonate Docker. Icon IDs are the three declared
// in plugin.json.
//
// Icons are set ONLY on groups. The instances inherit, which is what makes icon resolution visible
// in the end-to-end test: an instance under "alpha" must resolve to "left", never to the root
// group's "root".
var layout = []branch{
	{
		parent: "",
		children: []node{
			{ID: "fake", Kind: "group", Label: "Fake resources", IconID: "root", Order: 1,
				Actions: groupActions, DefaultActionID: actionRescan},
		},
	},
	{
		parent: "fake",
		children: []node{
			{ID: "alpha", Kind: "group", Label: "Alpha", IconID: "left", Order: 1,
				Actions: groupActions, DefaultActionID: actionRescan},
			{ID: "beta", Kind: "group", Label: "Beta", IconID: "right", Order: 2,
				Actions: groupActions, DefaultActionID: actionRescan},
		},
	},
	{
		parent:   "alpha",
		children: instances("alpha", 4),
	},
	{
		parent:   "beta",
		children: instances("beta", 3),
	},
}

// initialTones seeds every tone the model allows, including two nodes with no status at all.
var initialTones = map[string]string{
	"alpha-1": "ok",
	"alpha-2": "warn",
	"alpha-3": "error",
	"alpha-4": toneNone,
	"beta-1":  "busy",
	"beta-2":  "neutral",
	"beta-3":  toneNone,
}

var tooltips = map[string]string{
	"ok":      "healthy",
	"warn":    "needs attention",
	"error":   "failed",
	"busy":    "working",
	"neutral": "no opinion",
}

func instances(prefix string, count int) []node {
	out := make([]node, 0, count)
	for i := 1; i <= count; i++ {
		out = append(out, node{
			ID:              indexedID(prefix, i),
			Kind:            "instance",
			Label:           indexedID(prefix, i),
			Order:           i,
			Actions:         instanceActions,
			DefaultActionID: actionInspect,
		})
	}
	return out
}

func indexedID(prefix string, i int) string {
	return prefix + "-" + strconv.Itoa(i)
}

// parentOf maps a node ID to the branch it lives in, so an action on a node knows which branch to
// republish.
func parentOf(nodeID string) (string, bool) {
	for _, b := range layout {
		for _, child := range b.children {
			if child.ID == nodeID {
				return b.parent, true
			}
		}
	}
	return "", false
}
