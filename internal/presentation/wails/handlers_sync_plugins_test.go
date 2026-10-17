package wails

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

type fakePluginOffers struct {
	offers    []usecase.PluginOffer
	installed []usecase.OfferInstallRequest
	kept      []string
	previewed []string
	err       error
}

func (f *fakePluginOffers) Offers(context.Context) ([]usecase.PluginOffer, error) {
	return f.offers, f.err
}
func (f *fakePluginOffers) PreviewOffer(_ context.Context, id string) (usecase.GitHubPluginPreviewDTO, error) {
	f.previewed = append(f.previewed, id)
	return usecase.GitHubPluginPreviewDTO{ID: id, ReleaseTag: "v1.0.0"}, f.err
}
func (f *fakePluginOffers) InstallOffer(_ context.Context, req usecase.OfferInstallRequest) error {
	f.installed = append(f.installed, req)
	return f.err
}
func (f *fakePluginOffers) KeepPlugin(_ context.Context, id string) error {
	f.kept = append(f.kept, id)
	return f.err
}

type fakePluginData struct {
	status  []usecase.PluginDataStatus
	toggled map[string]bool
	err     error
}

func (f *fakePluginData) Status() ([]usecase.PluginDataStatus, error) { return f.status, f.err }
func (f *fakePluginData) SetSyncEnabled(_ context.Context, id string, on bool) error {
	if f.toggled == nil {
		f.toggled = map[string]bool{}
	}
	f.toggled[id] = on
	return f.err
}

func pluginSyncAPI() (*AppAPI, *fakePluginOffers, *fakePluginData) {
	a := &AppAPI{}
	offers, data := &fakePluginOffers{}, &fakePluginData{}
	a.SetPluginSyncServices(offers, data)
	return a, offers, data
}

func TestPluginSyncHandlersWithoutSyncAreUnavailable(t *testing.T) {
	a := &AppAPI{}
	if _, err := a.ListSyncPluginOffers(); !errors.Is(err, errSyncUnavailable) {
		t.Fatalf("offers without wiring: %v", err)
	}
	if _, err := a.InstallSyncPluginOffer(SyncPluginInstallRequest{PluginID: "com.example.echo"}); !errors.Is(err, errSyncUnavailable) {
		t.Fatalf("install without wiring: %v", err)
	}
	if _, err := a.ListSyncPluginData(); !errors.Is(err, errSyncUnavailable) {
		t.Fatalf("data without wiring: %v", err)
	}
	if err := a.SetSyncPluginData("com.example.echo", false); !errors.Is(err, errSyncUnavailable) {
		t.Fatalf("toggle without wiring: %v", err)
	}
}

func TestPluginSyncHandlersRefuseAnInvalidPluginID(t *testing.T) {
	a, offers, data := pluginSyncAPI()
	for _, id := range []string{"", "../vault", "com.example/echo", strings.Repeat("a", 300)} {
		if _, err := a.InstallSyncPluginOffer(SyncPluginInstallRequest{PluginID: id}); !errors.Is(err, errInvalidSyncRequest) {
			t.Errorf("install %q: %v, want errInvalidSyncRequest", id, err)
		}
		if _, err := a.PreviewSyncPluginOffer(id); !errors.Is(err, errInvalidSyncRequest) {
			t.Errorf("preview %q: %v", id, err)
		}
		if err := a.KeepSyncPlugin(id); !errors.Is(err, errInvalidSyncRequest) {
			t.Errorf("keep %q: %v", id, err)
		}
		if err := a.SetSyncPluginData(id, true); !errors.Is(err, errInvalidSyncRequest) {
			t.Errorf("toggle %q: %v", id, err)
		}
	}
	if len(offers.installed)+len(offers.kept)+len(offers.previewed)+len(data.toggled) != 0 {
		t.Fatal("an invalid plugin id reached the use case")
	}
}

func TestListSyncPluginOffersMapsEveryState(t *testing.T) {
	a, offers, _ := pluginSyncAPI()
	offers.offers = []usecase.PluginOffer{
		{Plugin: domain.SyncPlugin{ID: "com.example.a", Version: "1.0.0",
			Source: domain.SyncPluginSource{Kind: domain.PluginSourceGitHub, Repo: "example/a", Tag: "v1.0.0"}}, State: usecase.PluginOfferInstall},
		{Plugin: domain.SyncPlugin{ID: "com.example.b", Version: "0.1.0", Source: domain.SyncPluginSource{Kind: domain.PluginSourceManual}},
			State: usecase.PluginOfferManual},
	}
	got, err := a.ListSyncPluginOffers()
	if err != nil {
		t.Fatal(err)
	}
	want := []SyncPluginOfferDTO{
		{PluginID: "com.example.a", Version: "1.0.0", State: "install", Repo: "example/a", Tag: "v1.0.0"},
		{PluginID: "com.example.b", Version: "0.1.0", State: "manual"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("offers = %+v, want %+v", got, want)
	}
}

func TestInstallSyncPluginOfferPassesConsentsAndReportsAMismatch(t *testing.T) {
	a, offers, _ := pluginSyncAPI()
	res, err := a.InstallSyncPluginOffer(SyncPluginInstallRequest{PluginID: "com.example.echo", Exec: true, Network: true})
	if err != nil || res.Status != "installed" {
		t.Fatalf("install = %+v, %v", res, err)
	}
	want := usecase.OfferInstallRequest{PluginID: "com.example.echo",
		Consents: domainplugin.InstallConsents{Exec: true, ArbitraryNetwork: true}}
	if len(offers.installed) != 1 || offers.installed[0] != want {
		t.Fatalf("use case got %+v, want %+v: every consent the dialog collected, and only those", offers.installed, want)
	}

	offers.err = usecase.ErrPluginDigestMismatch
	res, err = a.InstallSyncPluginOffer(SyncPluginInstallRequest{PluginID: "com.example.echo"})
	if err != nil || res.Status != "mismatch" {
		t.Fatalf("a mismatching build = %+v, %v; the dialog asks whether to keep it, it is not an error", res, err)
	}
}

func TestInstallSyncPluginOfferMapsOfferErrors(t *testing.T) {
	a, offers, _ := pluginSyncAPI()
	for _, c := range []struct {
		err  error
		want string
	}{
		{usecase.ErrPluginOfferManual, "install it manually"},
		{usecase.ErrPluginOfferNotFound, "no longer offered"},
		{domain.ErrVaultLocked, "Unlock the vault"},
	} {
		offers.err = c.err
		_, err := a.InstallSyncPluginOffer(SyncPluginInstallRequest{PluginID: "com.example.echo"})
		if err == nil || !strings.Contains(err.Error(), c.want) || !errors.Is(err, c.err) {
			t.Errorf("%v mapped to %v; want a sentence containing %q that still unwraps to the cause", c.err, err, c.want)
		}
	}
}

func TestPluginSyncDataListAndToggle(t *testing.T) {
	a, _, data := pluginSyncAPI()
	data.status = []usecase.PluginDataStatus{{Plugin: "com.example.notes", Name: "Notes", Enabled: true, Waiting: 2, Files: 3}}
	got, err := a.ListSyncPluginData()
	if err != nil {
		t.Fatal(err)
	}
	if want := []SyncPluginDataDTO{{PluginID: "com.example.notes", Name: "Notes", Enabled: true, Waiting: 2, Files: 3}}; !slices.Equal(got, want) {
		t.Fatalf("data = %+v, want %+v", got, want)
	}
	if err := a.SetSyncPluginData("com.example.notes", false); err != nil {
		t.Fatal(err)
	}
	if on, ok := data.toggled["com.example.notes"]; !ok || on {
		t.Fatalf("toggled = %v; turning data sync off on this device", data.toggled)
	}
}

func TestKeepAndPreviewReachTheUseCase(t *testing.T) {
	a, offers, _ := pluginSyncAPI()
	if err := a.KeepSyncPlugin("com.example.echo"); err != nil {
		t.Fatal(err)
	}
	p, err := a.PreviewSyncPluginOffer("com.example.echo")
	if err != nil || p.ID != "com.example.echo" || p.ReleaseTag != "v1.0.0" {
		t.Fatalf("preview = %+v, %v", p, err)
	}
	if !slices.Equal(offers.kept, []string{"com.example.echo"}) || !slices.Equal(offers.previewed, []string{"com.example.echo"}) {
		t.Fatalf("kept %v, previewed %v", offers.kept, offers.previewed)
	}
}

func TestPluginDataReportPayloadCarriesOnlyNames(t *testing.T) {
	p := pluginDataReportPayload(usecase.PluginDataReport{Plugin: "com.example.notes", Pushed: 1, Applied: 2,
		Conflicts: []string{"state.json"}, Skipped: []string{"big.db"}})
	want := SyncPluginDataReportDTO{PluginID: "com.example.notes", Conflicts: []string{"state.json"}, Skipped: []string{"big.db"}}
	if p.PluginID != want.PluginID || !slices.Equal(p.Conflicts, want.Conflicts) || !slices.Equal(p.Skipped, want.Skipped) {
		t.Fatalf("payload = %+v, want %+v", p, want)
	}
	if !pluginDataReportWorthShowing(usecase.PluginDataReport{Conflicts: []string{"a"}}) ||
		pluginDataReportWorthShowing(usecase.PluginDataReport{Pushed: 4, Applied: 1}) {
		t.Fatal("only conflicts and skipped files are worth a notification; routine pushes are not")
	}
}

func TestRestartSyncPluginValidatesBeforeReachingThePlugins(t *testing.T) {
	a := &AppAPI{}
	if err := a.RestartSyncPlugin("../x"); !errors.Is(err, errInvalidSyncRequest) {
		t.Fatalf("restart of an invalid id: %v", err)
	}
	if err := a.RestartSyncPlugin("com.example.notes"); !errors.Is(err, errPluginManagerUnavailable) {
		t.Fatalf("restart without a plugin manager: %v", err)
	}
}

func TestEachInstallConsentMapsToItsOwnGrant(t *testing.T) {
	cases := map[string]struct {
		req  SyncPluginInstallRequest
		want domainplugin.InstallConsents
	}{
		"secret":       {SyncPluginInstallRequest{Secret: true}, domainplugin.InstallConsents{SecretAccess: true}},
		"auth":         {SyncPluginInstallRequest{Auth: true}, domainplugin.InstallConsents{AuthProvider: true}},
		"tunnel":       {SyncPluginInstallRequest{Tunnel: true}, domainplugin.InstallConsents{TunnelProvider: true}},
		"multiSession": {SyncPluginInstallRequest{MultiSession: true}, domainplugin.InstallConsents{MultiSession: true}},
		"network":      {SyncPluginInstallRequest{Network: true}, domainplugin.InstallConsents{ArbitraryNetwork: true}},
		"exec":         {SyncPluginInstallRequest{Exec: true}, domainplugin.InstallConsents{Exec: true}},
		"none":         {SyncPluginInstallRequest{}, domainplugin.InstallConsents{}},
	}
	for name, c := range cases {
		a, offers, _ := pluginSyncAPI()
		c.req.PluginID = "com.example.echo"
		if _, err := a.InstallSyncPluginOffer(c.req); err != nil {
			t.Fatal(err)
		}
		if got := offers.installed[0].Consents; got != c.want {
			t.Errorf("%s: consents %+v, want %+v; a consent the user did not give is never granted", name, got, c.want)
		}
	}
}
