// Command tukan-server is Tukan's Concord plugin process ("server mode",
// integration phase 4): it speaks Concord's plugin wire protocol and hosts
// Tukan boards for remote viewers, one independent BoardModel per connected
// human, rendered as plain strings over the wire — Concord's client has no
// Tukan-specific code at all. See "Tukan - Concord Plugin Integration.md"
// for the full protocol and the approved server-mode plan for the
// architecture this implements.
package main

import (
	"log"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/JMThomas00/tukan/internal/config"
	"github.com/JMThomas00/tukan/internal/database"
	"github.com/JMThomas00/tukan/internal/pluginserver"
	"github.com/JMThomas00/tukan/internal/styles"
	"github.com/JMThomas00/tukan/internal/themes"
	"github.com/JMThomas00/tukan/internal/wire"
)

func main() {
	wsURL := os.Getenv("CONCORD_WS_URL")
	pluginID := os.Getenv("CONCORD_PLUGIN_ID")
	token := os.Getenv("CONCORD_PLUGIN_TOKEN")
	if wsURL == "" || pluginID == "" || token == "" {
		log.Fatal("tukan-server: CONCORD_WS_URL, CONCORD_PLUGIN_ID, and CONCORD_PLUGIN_TOKEN must all be set")
	}

	fileCfg, err := config.LoadFileConfig()
	if err != nil {
		log.Fatalf("tukan-server: load config: %v", err)
	}
	theme, err := themes.GetTheme(fileCfg.Theme)
	if err != nil {
		theme = themes.GetDefaultTheme()
	}
	styles.Apply(theme)

	// Lip Gloss auto-detects color support by checking whether stdout is a
	// TTY. tukan-server's stdout is piped into Concord's own logger (see
	// process.go's cmd.Stdout redirection) — never a real terminal — so
	// that auto-detection sees a pipe and silently strips every ANSI color
	// code, rendering styles.Apply's theme (and the card form's
	// reverse-video focus cursor, which relies on the same styling) as
	// flat, uncolored text. The rendered frame is a string sent over the
	// wire, not written to this process's own terminal, so what matters is
	// Concord's real client terminal's capability, not tukan-server's —
	// forcing TrueColor here is correct until the wire protocol carries a
	// viewer's actual color capability (out of scope for now, see the
	// theme-deferral gap in the integration plan).
	lipgloss.SetColorProfile(termenv.TrueColor)

	// Same %APPDATA%\tukan\tukan.db standalone Tukan uses — a board
	// created through server mode is immediately visible in the local TUI
	// and vice versa, no data duplication between the two.
	cfg := config.Default()
	if err := os.MkdirAll(cfg.DBDir(), 0o755); err != nil {
		log.Fatalf("tukan-server: create data directory: %v", err)
	}
	db, err := database.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("tukan-server: open database: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		log.Fatalf("tukan-server: migrate: %v", err)
	}

	client, err := wire.Dial(wsURL)
	if err != nil {
		log.Fatalf("tukan-server: dial %s: %v", wsURL, err)
	}
	defer client.Close()
	if err := client.Identify(token); err != nil {
		log.Fatalf("tukan-server: identify: %v", err)
	}
	log.Printf("tukan-server: connected to %s as plugin %s", wsURL, pluginID)

	srv := pluginserver.New(client, db, pluginID, fileCfg.Theme)
	if err := srv.Run(); err != nil {
		log.Fatalf("tukan-server: %v", err)
	}
}
