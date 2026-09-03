// Package phrasedigest sends the Phrase Digest email to users who are due for one.
package phrasedigest

import (
	"context"
	"log/slog"
	"time"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/andreistefanciprian/phrasely/internal/email"
)

// minElapsed is the minimum time that must have passed since the last send
// for each frequency level.
var minElapsed = map[string]time.Duration{
	"daily":   23*time.Hour + 45*time.Minute,
	"weekly":  7*24*time.Hour - 15*time.Minute,
	"monthly": 30*24*time.Hour - 15*time.Minute,
}

// Service sends the Phrase Digest email to users who are due for one.
type Service struct {
	store  db.Store
	mailer email.Sender
}

func NewService(store db.Store, mailer email.Sender) *Service {
	return &Service{store: store, mailer: mailer}
}

// SendDue iterates all recipients with a non-disabled frequency and sends
// to those whose elapsed time exceeds their chosen interval.
// Invocation time is controlled externally by the scheduler.
func (s *Service) SendDue(ctx context.Context) error {
	now := time.Now().UTC()

	recipients, err := s.store.ListDigestRecipients(ctx)
	if err != nil {
		return err
	}
	for _, r := range recipients {
		if !isDue(r, now) {
			continue
		}
		s.sendToUser(ctx, r, now)
	}
	return nil
}

func isDue(r db.DigestRecipient, now time.Time) bool {
	threshold, ok := minElapsed[r.Frequency]
	if !ok {
		return false
	}
	if r.LastSentAt == nil {
		return true
	}
	return now.Sub(*r.LastSentAt) >= threshold
}

func (s *Service) sendToUser(ctx context.Context, r db.DigestRecipient, now time.Time) {
	summaries, err := s.store.GetRandomPhrases(ctx, r.UserID, 1)
	if err != nil {
		slog.Error("phrase digest: get random phrases failed", "user_id", r.UserID, "error", err)
		return
	}
	if len(summaries) == 0 {
		slog.Debug("phrase digest: user has no phrases, skipping", "user_id", r.UserID)
		return
	}

	phrases := make([]email.DigestPhrase, len(summaries))
	for i, p := range summaries {
		phrases[i] = email.DigestPhrase{
			ID:        p.ID,
			Headwords: p.Headwords,
			Phrase:    p.Phrase,
		}
	}

	if err := s.mailer.SendPhraseDigest(r.Email, phrases); err != nil {
		slog.Error("phrase digest send failed", "user_id", r.UserID, "email", r.Email, "error", err)
		return
	}

	if err := s.store.MarkDigestSent(ctx, r.UserID, now); err != nil {
		slog.Error("phrase digest mark sent failed", "user_id", r.UserID, "error", err)
		return
	}

	slog.Info("phrase digest sent", "user_id", r.UserID, "email", r.Email, "count", len(phrases))
}
