package ipc

import (
	"net/http"
	"testing"
)

func FuzzExtractSubPath(f *testing.F) {
	f.Add("/api/type/ArchiSteamFarm.Steam.Storage.BotConfig")
	f.Add("/api/structure/ArchiSteamFarm.Storage.GlobalConfig")
	f.Add("/api/type/")
	f.Add("/api/type/%2B")
	f.Add("")
	f.Add("/api/type/a/b/c")
	f.Add("/api/type/%00%01%ff")
	f.Add("/api/type/..%2f..")

	f.Fuzz(func(t *testing.T, path string) {
		r, err := http.NewRequest(http.MethodGet, path, nil)
		if err != nil {
			return
		}
		extractSubPath(r, "/api/type/")
	})
}
