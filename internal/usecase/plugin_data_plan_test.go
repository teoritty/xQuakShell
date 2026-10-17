package usecase

import "testing"

func TestPlanPluginFile(t *testing.T) {
	cases := []struct {
		name               string
		disk, mirror, last string
		mayWrite           bool
		want               pluginFileAction
		conflict           bool
	}{
		{"in step", "a", "a", "a", true, actionNone, false},
		{"never seen anywhere", "", "", "", true, actionNone, false},
		{"edited here", "b", "a", "a", true, actionPush, false},
		{"created here", "b", "", "", true, actionPush, false},
		{"deleted here", "", "a", "a", true, actionPush, false},
		{"edited elsewhere, stopped", "a", "b", "a", true, actionWrite, false},
		{"edited elsewhere, running", "a", "b", "a", false, actionWait, false},
		{"created elsewhere", "", "b", "", true, actionWrite, false},
		{"deleted elsewhere", "a", "", "a", true, actionWrite, false},
		{"edited on both", "c", "b", "a", true, actionPush, true},
		{"edited on both, same result", "b", "b", "a", true, actionPush, false},
		{"edited here, deleted elsewhere", "c", "", "a", true, actionPush, true},
		{"first look, files agree", "a", "a", "", true, actionPush, false},
	}
	for _, c := range cases {
		got, conflict := planPluginFile(c.disk, c.mirror, c.last, c.mayWrite)
		if got != c.want || conflict != c.conflict {
			t.Errorf("%s: plan = %v, conflict %v; want %v, conflict %v", c.name, got, conflict, c.want, c.conflict)
		}
	}
}
