// Command send-phrase-digest sends the Phrase Digest email to every user who
// is due for one. Intended to run as a Railway cron job, hourly.
package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"
	_ "time/tzdata" // embed IANA timezone database — scratch image has none

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/andreistefanciprian/phrasely/internal/email"
	"github.com/andreistefanciprian/phrasely/internal/phrasedigest"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(os.Getenv("LOG_LEVEL")),
	})))

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		slog.Error("DATABASE_URL is not set")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := db.NewPostgresStore(ctx, dsn)
	if err != nil {
		slog.Error("could not connect to database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	var mailer email.Sender
	resendKey := os.Getenv("RESEND_API_KEY")
	emailFrom := os.Getenv("EMAIL_FROM")
	if resendKey != "" && emailFrom != "" {
		mailer = email.NewResendSender(resendKey, emailFrom)
	} else {
		mailer = &email.LogSender{}
		slog.Warn("RESEND_API_KEY or EMAIL_FROM not set — digest logged to stdout")
	}

	svc := phrasedigest.NewService(store, mailer)

	runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer runCancel()

	if err := svc.SendDue(runCtx); err != nil {
		slog.Error("send phrase digest failed", "error", err)
		os.Exit(1)
	}
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
