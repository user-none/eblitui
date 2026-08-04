package desktop

import (
	"testing"

	"github.com/user-none/eblitui/desktop/storage"
)

func assignConfig() *storage.InputConfig {
	return &storage.InputConfig{
		Profiles: []storage.ControllerProfile{
			{ID: "ps-default", SDLID: "sdl-ps", Controller: "PlayStation", Name: "default"},
			{ID: "ps-m30", SDLID: "sdl-ps", Controller: "PlayStation", Name: "M30"},
			{ID: "xb-default", SDLID: "sdl-xb", Controller: "Xbox", Name: "default"},
		},
		Players: []storage.PlayerConfig{
			{Profile: "ps-default"},
			{Profile: "ps-m30"},
		},
	}
}

func TestPlayerAssignmentBinding(t *testing.T) {
	t.Run("binds matching pads lowest player first", func(t *testing.T) {
		pa := NewPlayerAssignment(2)
		cfg := assignConfig()
		pads := []PadInfo{
			{ID: 0, SDLID: "sdl-ps", Name: "PlayStation"},
			{ID: 1, SDLID: "sdl-ps", Name: "PlayStation"},
		}

		events := pa.Update(pads, cfg)
		if len(events) != 2 {
			t.Fatalf("expected 2 events, got %d", len(events))
		}
		if id, ok := pa.PadFor(0); !ok || id != 0 {
			t.Errorf("player 1 should be bound to pad 0, got %v %v", id, ok)
		}
		if id, ok := pa.PadFor(1); !ok || id != 1 {
			t.Errorf("player 2 should be bound to pad 1, got %v %v", id, ok)
		}
	})

	t.Run("non-matching pad stays unbound", func(t *testing.T) {
		pa := NewPlayerAssignment(2)
		cfg := assignConfig()
		pads := []PadInfo{{ID: 0, SDLID: "sdl-other", Name: "Generic"}}

		events := pa.Update(pads, cfg)
		if len(events) != 0 {
			t.Fatalf("expected no events, got %v", events)
		}
		if _, ok := pa.PadFor(0); ok {
			t.Error("player 1 should be unbound")
		}
	})

	t.Run("mixed models bind by profile key not order", func(t *testing.T) {
		pa := NewPlayerAssignment(2)
		cfg := assignConfig()
		cfg.Players[1].Profile = "xb-default"
		// Xbox pad enumerates first but belongs to player 2.
		pads := []PadInfo{
			{ID: 0, SDLID: "sdl-xb", Name: "Xbox"},
			{ID: 1, SDLID: "sdl-ps", Name: "PlayStation"},
		}

		pa.Update(pads, cfg)
		if id, ok := pa.PadFor(0); !ok || id != 1 {
			t.Errorf("player 1 should be bound to the PlayStation pad, got %v %v", id, ok)
		}
		if id, ok := pa.PadFor(1); !ok || id != 0 {
			t.Errorf("player 2 should be bound to the Xbox pad, got %v %v", id, ok)
		}
	})

	t.Run("later connection binds remaining player", func(t *testing.T) {
		pa := NewPlayerAssignment(2)
		cfg := assignConfig()

		pa.Update([]PadInfo{{ID: 0, SDLID: "sdl-ps", Name: "PlayStation"}}, cfg)
		if _, ok := pa.PadFor(1); ok {
			t.Fatal("player 2 should be unbound with one pad")
		}

		events := pa.Update([]PadInfo{
			{ID: 0, SDLID: "sdl-ps", Name: "PlayStation"},
			{ID: 1, SDLID: "sdl-ps", Name: "PlayStation"},
		}, cfg)
		if len(events) != 1 || events[0].Player != 1 || !events[0].Bound {
			t.Fatalf("expected player 2 bind event, got %v", events)
		}
		if id, ok := pa.PadFor(0); !ok || id != 0 {
			t.Error("player 1 binding should be unchanged")
		}
	})

	t.Run("unassigned player never binds", func(t *testing.T) {
		pa := NewPlayerAssignment(2)
		cfg := assignConfig()
		cfg.Players[1].Profile = ""
		pads := []PadInfo{
			{ID: 0, SDLID: "sdl-ps", Name: "PlayStation"},
			{ID: 1, SDLID: "sdl-ps", Name: "PlayStation"},
		}

		pa.Update(pads, cfg)
		if _, ok := pa.PadFor(1); ok {
			t.Error("player 2 has no profile and should stay unbound")
		}
	})

	t.Run("player count gates slots", func(t *testing.T) {
		pa := NewPlayerAssignment(1)
		cfg := assignConfig()
		pads := []PadInfo{
			{ID: 0, SDLID: "sdl-ps", Name: "PlayStation"},
			{ID: 1, SDLID: "sdl-ps", Name: "PlayStation"},
		}

		pa.Update(pads, cfg)
		if _, ok := pa.PadFor(0); !ok {
			t.Error("player 1 should be bound")
		}
		if _, ok := pa.PadFor(1); ok {
			t.Error("player 2 slot should not exist")
		}
	})
}

func TestPlayerAssignmentUnbinding(t *testing.T) {
	t.Run("disconnect unbinds", func(t *testing.T) {
		pa := NewPlayerAssignment(2)
		cfg := assignConfig()
		pa.Update([]PadInfo{{ID: 0, SDLID: "sdl-ps", Name: "PlayStation"}}, cfg)

		events := pa.Update(nil, cfg)
		if len(events) != 1 || events[0].Player != 0 || events[0].Bound {
			t.Fatalf("expected player 1 unbind event, got %v", events)
		}
		if _, ok := pa.PadFor(0); ok {
			t.Error("player 1 should be unbound after disconnect")
		}
	})

	t.Run("reconnect rebinds", func(t *testing.T) {
		pa := NewPlayerAssignment(2)
		cfg := assignConfig()
		pads := []PadInfo{{ID: 0, SDLID: "sdl-ps", Name: "PlayStation"}}
		pa.Update(pads, cfg)
		pa.Update(nil, cfg)

		events := pa.Update(pads, cfg)
		if len(events) != 1 || events[0].Player != 0 || !events[0].Bound {
			t.Fatalf("expected player 1 bind event, got %v", events)
		}
	})

	t.Run("profile change to different model unbinds", func(t *testing.T) {
		pa := NewPlayerAssignment(2)
		cfg := assignConfig()
		pads := []PadInfo{{ID: 0, SDLID: "sdl-ps", Name: "PlayStation"}}
		pa.Update(pads, cfg)

		cfg.Players[0].Profile = "xb-default"
		events := pa.Update(pads, cfg)
		if len(events) != 1 || events[0].Bound {
			t.Fatalf("expected unbind event, got %v", events)
		}
	})

	t.Run("profile change within model keeps binding", func(t *testing.T) {
		pa := NewPlayerAssignment(2)
		cfg := assignConfig()
		pads := []PadInfo{{ID: 0, SDLID: "sdl-ps", Name: "PlayStation"}}
		pa.Update(pads, cfg)

		cfg.Players[0].Profile = "ps-m30"
		events := pa.Update(pads, cfg)
		if len(events) != 0 {
			t.Fatalf("expected no events, got %v", events)
		}
		if id, ok := pa.PadFor(0); !ok || id != 0 {
			t.Error("player 1 should keep its pad")
		}
	})

	t.Run("profile deletion unbinds", func(t *testing.T) {
		pa := NewPlayerAssignment(2)
		cfg := assignConfig()
		pads := []PadInfo{{ID: 0, SDLID: "sdl-ps", Name: "PlayStation"}}
		pa.Update(pads, cfg)

		cfg.Players[0].Profile = ""
		events := pa.Update(pads, cfg)
		if len(events) != 1 || events[0].Bound {
			t.Fatalf("expected unbind event, got %v", events)
		}
	})

	t.Run("pad id reuse by different model rebinds correctly", func(t *testing.T) {
		pa := NewPlayerAssignment(2)
		cfg := assignConfig()
		cfg.Players[1].Profile = "xb-default"
		pa.Update([]PadInfo{{ID: 0, SDLID: "sdl-ps", Name: "PlayStation"}}, cfg)

		// Pad 0 is replaced by an Xbox pad reusing the same ID.
		events := pa.Update([]PadInfo{{ID: 0, SDLID: "sdl-xb", Name: "Xbox"}}, cfg)
		if len(events) != 2 {
			t.Fatalf("expected unbind + bind events, got %v", events)
		}
		if _, ok := pa.PadFor(0); ok {
			t.Error("player 1 should be unbound")
		}
		if id, ok := pa.PadFor(1); !ok || id != 0 {
			t.Error("player 2 should be bound to the reused pad ID")
		}
	})
}

func TestAutoCreateFirstProfile(t *testing.T) {
	t.Run("creates and assigns on fresh config", func(t *testing.T) {
		input := &storage.InputConfig{}
		pads := []PadInfo{{ID: 0, SDLID: "sdl-ps", Name: "PlayStation"}}

		if !AutoCreateFirstProfile(input, pads) {
			t.Fatal("expected auto-create to fire")
		}
		if len(input.Profiles) != 1 {
			t.Fatalf("expected 1 profile, got %d", len(input.Profiles))
		}
		p := input.Profiles[0]
		if p.SDLID != "sdl-ps" || p.Controller != "PlayStation" || p.Name != "default" {
			t.Errorf("unexpected profile: %v", p)
		}
		if p.ID == "" {
			t.Error("profile ID should be generated")
		}
		if len(input.Players) == 0 || input.Players[0].Profile != p.ID {
			t.Error("player 1 should be assigned the new profile")
		}
	})

	t.Run("does not fire with no pads", func(t *testing.T) {
		input := &storage.InputConfig{}
		if AutoCreateFirstProfile(input, nil) {
			t.Error("should not fire without a pad")
		}
	})

	t.Run("does not fire when any profile exists", func(t *testing.T) {
		input := &storage.InputConfig{
			Profiles: []storage.ControllerProfile{
				{ID: "p1", SDLID: "sdl-xb", Controller: "Xbox", Name: "default"},
			},
		}
		pads := []PadInfo{{ID: 0, SDLID: "sdl-ps", Name: "PlayStation"}}
		if AutoCreateFirstProfile(input, pads) {
			t.Error("should not fire when profiles exist")
		}
	})

	t.Run("fires only once", func(t *testing.T) {
		input := &storage.InputConfig{}
		pads := []PadInfo{{ID: 0, SDLID: "sdl-ps", Name: "PlayStation"}}
		if !AutoCreateFirstProfile(input, pads) {
			t.Fatal("first call should fire")
		}
		if AutoCreateFirstProfile(input, pads) {
			t.Error("second call should not fire")
		}
	})
}

func TestNewProfileID(t *testing.T) {
	input := &storage.InputConfig{}
	id := input.NewProfileID()
	if len(id) != 8 {
		t.Errorf("expected 8-char id, got %q", id)
	}
	input.Profiles = append(input.Profiles, storage.ControllerProfile{ID: id, SDLID: "s", Controller: "c", Name: "n"})
	id2 := input.NewProfileID()
	if id2 == id {
		t.Error("ids should be unique")
	}
}
