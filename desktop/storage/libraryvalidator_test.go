package storage

import "testing"

func TestSanitizeLibraryEntries(t *testing.T) {
	t.Run("negative playTimeSeconds set to 0", func(t *testing.T) {
		lib := DefaultLibrary()
		lib.AddGame(&GameEntry{CRC32: "1", PlayTimeSeconds: -100})

		SanitizeLibraryEntries(lib)

		if lib.Games["1"].PlayTimeSeconds != 0 {
			t.Errorf("expected 0, got %d", lib.Games["1"].PlayTimeSeconds)
		}
	})

	t.Run("valid playTimeSeconds preserved", func(t *testing.T) {
		lib := DefaultLibrary()
		lib.AddGame(&GameEntry{CRC32: "1", PlayTimeSeconds: 500})

		SanitizeLibraryEntries(lib)

		if lib.Games["1"].PlayTimeSeconds != 500 {
			t.Errorf("expected 500, got %d", lib.Games["1"].PlayTimeSeconds)
		}
	})

	t.Run("zero playTimeSeconds preserved", func(t *testing.T) {
		lib := DefaultLibrary()
		lib.AddGame(&GameEntry{CRC32: "1", PlayTimeSeconds: 0})

		SanitizeLibraryEntries(lib)

		if lib.Games["1"].PlayTimeSeconds != 0 {
			t.Errorf("expected 0, got %d", lib.Games["1"].PlayTimeSeconds)
		}
	})

	t.Run("negative lastPlayed set to 0", func(t *testing.T) {
		lib := DefaultLibrary()
		lib.AddGame(&GameEntry{CRC32: "1", LastPlayed: -1})

		SanitizeLibraryEntries(lib)

		if lib.Games["1"].LastPlayed != 0 {
			t.Errorf("expected 0, got %d", lib.Games["1"].LastPlayed)
		}
	})

	t.Run("valid lastPlayed preserved", func(t *testing.T) {
		lib := DefaultLibrary()
		lib.AddGame(&GameEntry{CRC32: "1", LastPlayed: 1700000000})

		SanitizeLibraryEntries(lib)

		if lib.Games["1"].LastPlayed != 1700000000 {
			t.Errorf("expected 1700000000, got %d", lib.Games["1"].LastPlayed)
		}
	})

	t.Run("negative added set to 0", func(t *testing.T) {
		lib := DefaultLibrary()
		lib.AddGame(&GameEntry{CRC32: "1", Added: -50})

		SanitizeLibraryEntries(lib)

		if lib.Games["1"].Added != 0 {
			t.Errorf("expected 0, got %d", lib.Games["1"].Added)
		}
	})

	t.Run("valid added preserved", func(t *testing.T) {
		lib := DefaultLibrary()
		lib.AddGame(&GameEntry{CRC32: "1", Added: 1700000000})

		SanitizeLibraryEntries(lib)

		if lib.Games["1"].Added != 1700000000 {
			t.Errorf("expected 1700000000, got %d", lib.Games["1"].Added)
		}
	})

	t.Run("negative saveSlot set to 0", func(t *testing.T) {
		lib := DefaultLibrary()
		lib.AddGame(&GameEntry{CRC32: "1", Settings: GameSettings{SaveSlot: -1}})

		SanitizeLibraryEntries(lib)

		if lib.Games["1"].Settings.SaveSlot != 0 {
			t.Errorf("expected 0, got %d", lib.Games["1"].Settings.SaveSlot)
		}
	})

	t.Run("saveSlot above 9 set to 0", func(t *testing.T) {
		lib := DefaultLibrary()
		lib.AddGame(&GameEntry{CRC32: "1", Settings: GameSettings{SaveSlot: 10}})

		SanitizeLibraryEntries(lib)

		if lib.Games["1"].Settings.SaveSlot != 0 {
			t.Errorf("expected 0, got %d", lib.Games["1"].Settings.SaveSlot)
		}
	})

	t.Run("valid saveSlot boundaries preserved", func(t *testing.T) {
		lib := DefaultLibrary()
		lib.AddGame(&GameEntry{CRC32: "a", Settings: GameSettings{SaveSlot: 0}})
		lib.AddGame(&GameEntry{CRC32: "b", Settings: GameSettings{SaveSlot: 9}})
		lib.AddGame(&GameEntry{CRC32: "c", Settings: GameSettings{SaveSlot: 5}})

		SanitizeLibraryEntries(lib)

		if lib.Games["a"].Settings.SaveSlot != 0 {
			t.Errorf("slot 0: expected 0, got %d", lib.Games["a"].Settings.SaveSlot)
		}
		if lib.Games["b"].Settings.SaveSlot != 9 {
			t.Errorf("slot 9: expected 9, got %d", lib.Games["b"].Settings.SaveSlot)
		}
		if lib.Games["c"].Settings.SaveSlot != 5 {
			t.Errorf("slot 5: expected 5, got %d", lib.Games["c"].Settings.SaveSlot)
		}
	})

	t.Run("multiple fields fixed on same entry", func(t *testing.T) {
		lib := DefaultLibrary()
		lib.AddGame(&GameEntry{
			CRC32:           "1",
			PlayTimeSeconds: -10,
			LastPlayed:      -20,
			Added:           -30,
			Settings: GameSettings{
				SaveSlot: 99,
			},
		})

		SanitizeLibraryEntries(lib)

		game := lib.Games["1"]
		if game.PlayTimeSeconds != 0 {
			t.Errorf("playTimeSeconds: expected 0, got %d", game.PlayTimeSeconds)
		}
		if game.LastPlayed != 0 {
			t.Errorf("lastPlayed: expected 0, got %d", game.LastPlayed)
		}
		if game.Added != 0 {
			t.Errorf("added: expected 0, got %d", game.Added)
		}
		if game.Settings.SaveSlot != 0 {
			t.Errorf("saveSlot: expected 0, got %d", game.Settings.SaveSlot)
		}
	})

	t.Run("multiple entries sanitized", func(t *testing.T) {
		lib := DefaultLibrary()
		lib.AddGame(&GameEntry{CRC32: "1", PlayTimeSeconds: -1})
		lib.AddGame(&GameEntry{CRC32: "2", LastPlayed: -1})
		lib.AddGame(&GameEntry{CRC32: "3", PlayTimeSeconds: 100}) // valid

		SanitizeLibraryEntries(lib)

		if lib.Games["1"].PlayTimeSeconds != 0 {
			t.Errorf("game 1: expected 0, got %d", lib.Games["1"].PlayTimeSeconds)
		}
		if lib.Games["2"].LastPlayed != 0 {
			t.Errorf("game 2: expected 0, got %d", lib.Games["2"].LastPlayed)
		}
		if lib.Games["3"].PlayTimeSeconds != 100 {
			t.Errorf("game 3: expected 100, got %d", lib.Games["3"].PlayTimeSeconds)
		}
	})

	t.Run("empty library no panic", func(t *testing.T) {
		lib := DefaultLibrary()
		SanitizeLibraryEntries(lib) // should not panic
	})

	t.Run("nil games map no panic", func(t *testing.T) {
		lib := &Library{Games: nil}
		SanitizeLibraryEntries(lib) // should not panic
	})
}

func TestValidateLibrary(t *testing.T) {
	t.Run("valid library no errors", func(t *testing.T) {
		lib := DefaultLibrary()
		errs := ValidateLibrary(lib)
		if len(errs) != 0 {
			t.Errorf("expected no errors, got: %v", errs)
		}
	})

	t.Run("invalid version", func(t *testing.T) {
		lib := &Library{Version: 99}
		errs := ValidateLibrary(lib)
		if len(errs) != 1 {
			t.Errorf("expected 1 error, got %d: %v", len(errs), errs)
		}
	})

	t.Run("version 0 is invalid", func(t *testing.T) {
		lib := &Library{Version: 0}
		errs := ValidateLibrary(lib)
		if len(errs) != 1 {
			t.Errorf("expected 1 error, got %d: %v", len(errs), errs)
		}
	})

	t.Run("negative version", func(t *testing.T) {
		lib := &Library{Version: -1}
		errs := ValidateLibrary(lib)
		if len(errs) != 1 {
			t.Errorf("expected 1 error, got %d: %v", len(errs), errs)
		}
	})
}

func TestCorrectLibrary(t *testing.T) {
	t.Run("fixes invalid version", func(t *testing.T) {
		lib := &Library{Version: 99}
		corrected := CorrectLibrary(lib)
		if corrected.Version != 1 {
			t.Errorf("expected version 1, got %d", corrected.Version)
		}
	})

	t.Run("valid version preserved", func(t *testing.T) {
		lib := DefaultLibrary()
		corrected := CorrectLibrary(lib)
		if corrected.Version != 1 {
			t.Errorf("expected version 1, got %d", corrected.Version)
		}
	})
}

func TestCleanGameKey(t *testing.T) {
	tests := map[string]string{
		"1A2B3C4D":    "1A2B3C4D",
		"MK-81009":    "MK-81009",
		"T-1234G V1":  "T-1234G V1",
		"../../evil":  "....evil",
		"..\\evil":    "..evil",
		"a/b\\c":      "abc",
		"a\x00b\nc":   "abc",
		"..":          "",
		".":           "",
		"/../":        "",
		"":            "",
		"\x01\x02/\\": "",
	}
	for in, want := range tests {
		if got := CleanGameKey(in); got != want {
			t.Errorf("CleanGameKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeLibraryEntriesGameKeys(t *testing.T) {
	lib := &Library{Games: map[string]*GameEntry{
		"1A2B3C4D":   {CRC32: "1A2B3C4D"},
		"../evil":    {CRC32: "../evil"},
		"..":         {CRC32: ".."},
		"GOOD-KEY":   {CRC32: "../mismatch"},
		"NIL-ENTRY":  nil,
		"DUP/":       {CRC32: "DUP/"},
		"DUP":        {CRC32: "DUP"},
		"MK-81009\n": {CRC32: "MK-81009\n"},
	}}

	SanitizeLibraryEntries(lib)

	want := []string{"1A2B3C4D", "..evil", "GOOD-KEY", "DUP", "MK-81009"}
	if len(lib.Games) != len(want) {
		t.Errorf("got %d games, want %d: %v", len(lib.Games), len(want), lib.Games)
	}
	for _, key := range want {
		game, ok := lib.Games[key]
		if !ok {
			t.Errorf("missing key %q", key)
			continue
		}
		if game.CRC32 != key {
			t.Errorf("game %q has CRC32 %q, want %q", key, game.CRC32, key)
		}
	}
}
