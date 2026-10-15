package bot

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/asf-go/asf/internal/config"
	"github.com/asf-go/asf/internal/logging"
)

func FuzzParseGameIdentifier(f *testing.F) {
	f.Add("app/730")
	f.Add("sub/123")
	f.Add("https://store.steampowered.com/app/730/")
	f.Add("steam://launch/730")
	f.Add("a/730")
	f.Add("s/123")
	f.Add("730")
	f.Add("random-string")
	f.Add("")
	f.Add("!@#$%^&*()")

	f.Fuzz(func(t *testing.T, input string) {
		ParseGameIdentifier(input, GameIdentifierApplication)
	})
}

func FuzzHandleCommandText(f *testing.F) {
	dir, err := os.MkdirTemp("", "asf-fuzz-*")
	if err != nil {
		panic(err)
	}
	log, err := logging.New("Bot-fuzz", filepath.Join(dir, "fuzz.log"), logging.Info)
	if err != nil {
		panic(err)
	}
	b := &Bot{
		Name:         "fuzzbot",
		Config:       config.DefaultBotConfig(),
		Log:          log,
		CommandPrefix: "!",
		configDir:    dir,
		OwnerSteamID: 76561198000000001,
	}
	var mu sync.Mutex

	f.Add("!status")
	f.Add("!2fa")
	f.Add("!password secret")
	f.Add("!batch ABCDEF-GHIJKL-MNOPQR")
	f.Add("!unknown")
	f.Add("")
	f.Add("!!!")

	f.Fuzz(func(t *testing.T, message string) {
		mu.Lock()
		defer mu.Unlock()
		b.handleCommandText(76561198000000001, message)
	})
}