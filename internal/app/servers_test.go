package app

import "testing"

const sampleLink = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@example.com:443" +
	"?type=tcp&security=reality&pbk=ABCpublicKey&fp=chrome&sni=www.microsoft.com" +
	"&sid=0123abcd&spx=%2F&flow=xtls-rprx-vision#my-server"

func TestAddServerParsesAndSelectsFirst(t *testing.T) {
	svc, em, _, _ := testDeps(t)
	if err := svc.AddServer(sampleLink); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	st := svc.GetState()
	if len(st.Servers) != 1 {
		t.Fatalf("got %d servers, want 1", len(st.Servers))
	}
	if st.Servers[0].Host != "example.com" || st.Servers[0].Name != "my-server" {
		t.Errorf("server DTO wrong: %+v", st.Servers[0])
	}
	if st.ActiveServer != 0 {
		t.Errorf("first added server should become active, got %d", st.ActiveServer)
	}
	// A "state" event must have been emitted.
	if len(em.events) == 0 || em.events[len(em.events)-1].name != "state" {
		t.Error("AddServer should emit a state event")
	}
}

func TestAddServerRejectsBadLink(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	if err := svc.AddServer("not-a-vless-link"); err == nil {
		t.Error("expected error for malformed link")
	}
	if len(svc.GetState().Servers) != 0 {
		t.Error("bad link must not be stored")
	}
}

func TestAddServerPersists(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	if err := svc.AddServer(sampleLink); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	// A new service reading the same path must see the server.
	svc2, err := New(svc.deps)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(svc2.GetState().Servers) != 1 {
		t.Error("server was not persisted")
	}
}

func TestRemoveServerAdjustsActive(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	mustAdd(t, svc, sampleLink) // index 0
	mustAdd(t, svc, sampleLink) // index 1
	if err := svc.SetActiveServer(1); err != nil {
		t.Fatalf("SetActiveServer: %v", err)
	}
	if err := svc.RemoveServer(0); err != nil {
		t.Fatalf("RemoveServer: %v", err)
	}
	st := svc.GetState()
	if len(st.Servers) != 1 {
		t.Fatalf("got %d servers, want 1", len(st.Servers))
	}
	if st.ActiveServer != 0 {
		t.Errorf("active index should shift to 0 after removing index 0, got %d", st.ActiveServer)
	}
}

func TestRemoveLastServerClearsActive(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	mustAdd(t, svc, sampleLink)
	if err := svc.RemoveServer(0); err != nil {
		t.Fatalf("RemoveServer: %v", err)
	}
	if svc.GetState().ActiveServer != -1 {
		t.Error("removing the only server should reset ActiveServer to -1")
	}
}

func TestRemoveServerOutOfRange(t *testing.T) {
	svc, _, _, _ := testDeps(t)
	if err := svc.RemoveServer(0); err == nil {
		t.Error("expected error removing from empty list")
	}
}

func mustAdd(t *testing.T, svc *Service, link string) {
	t.Helper()
	if err := svc.AddServer(link); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
}
