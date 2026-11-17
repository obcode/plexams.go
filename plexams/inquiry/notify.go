package inquiry

import (
	"context"
	"sync"
	"time"

	"github.com/obcode/plexams.go/db/sqlc"
	"github.com/rs/zerolog/log"
)

// Mailer sends one notification mail about portal changes to one planner.
type Mailer interface {
	SendPortalActivityMail(ctx context.Context, to string, items []*Activity, daily bool) error
}

// Notifier mails the planners about portal changes.
//
// IMMEDIATE: after a change it waits until nothing new came for `quiet` (at most
// `maxWait` after the first one) and then sends each recipient one mail with
// everything since their last mail. A sweep every `sweep` catches what a restart
// swallowed. DAILY: Daily is called by the scheduler.
//
// The position (mailed_up_to) only moves after a successful send, so a failed mail is
// repeated by the next run.
type Notifier struct {
	store   Store
	mailer  Mailer
	quiet   time.Duration
	maxWait time.Duration
	sweep   time.Duration

	wake chan struct{}
	mu   sync.Mutex // one flush at a time
}

// NewNotifier builds a notifier with the production timings.
func NewNotifier(store Store, mailer Mailer) *Notifier {
	return &Notifier{store: store, mailer: mailer, quiet: 2 * time.Minute, maxWait: 10 * time.Minute,
		sweep: 5 * time.Minute, wake: make(chan struct{}, 1)}
}

// Wake signals a committed change. Never blocks.
func (n *Notifier) Wake() {
	select {
	case n.wake <- struct{}{}:
	default:
	}
}

// Run sends the immediate mails until ctx is cancelled.
func (n *Notifier) Run(ctx context.Context) {
	sweep := time.NewTicker(n.sweep)
	defer sweep.Stop()
	var quiet, deadline <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-n.wake:
			quiet = time.After(n.quiet)
			if deadline == nil {
				deadline = time.After(n.maxWait)
			}
		case <-quiet:
			quiet, deadline = nil, nil
			n.Flush(ctx, ModeImmediate)
		case <-deadline:
			quiet, deadline = nil, nil
			n.Flush(ctx, ModeImmediate)
		case <-sweep.C:
			if quiet == nil {
				n.Flush(ctx, ModeImmediate)
			}
		}
	}
}

// Daily sends the daily mails.
func (n *Notifier) Daily(ctx context.Context) { n.Flush(ctx, ModeDaily) }

// Flush mails every recipient with the given mode what is new for them. Returns the
// number of mails sent.
func (n *Notifier) Flush(ctx context.Context, mode string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	q := n.store.Q(ctx)
	recipients, err := q.ListNotificationRecipients(ctx)
	if err != nil {
		log.Error().Err(err).Msg("portal notifications: cannot list recipients")
		return 0
	}
	sent := 0
	for _, r := range recipients {
		if r.Mode != mode {
			continue
		}
		after := int64(0)
		if r.MailedUpTo != nil {
			after = *r.MailedUpTo
		} else {
			// No row yet (default DAILY): start now instead of mailing the whole history.
			maxID, err := q.MaxInquiryActivityID(ctx)
			if err != nil {
				log.Error().Err(err).Msg("portal notifications: cannot read the position")
				continue
			}
			if _, err := q.UpsertNotificationMode(ctx, sqlc.UpsertNotificationModeParams{Email: r.Email, Mode: r.Mode, StartAt: maxID}); err != nil {
				log.Error().Err(err).Msg("portal notifications: cannot store the position")
			}
			continue
		}
		rows, err := q.InquiryActivitySince(ctx, after)
		if err != nil {
			log.Error().Err(err).Msg("portal notifications: cannot read the activity")
			continue
		}
		if len(rows) == 0 {
			continue
		}
		items := make([]*Activity, 0, len(rows))
		for _, row := range rows {
			items = append(items, toActivity(row))
		}
		if err := n.mailer.SendPortalActivityMail(ctx, r.Email, items, mode == ModeDaily); err != nil {
			log.Error().Err(err).Str("mode", mode).Msg("portal notifications: cannot send mail")
			continue
		}
		if err := q.SetNotificationMailedUpTo(ctx, sqlc.SetNotificationMailedUpToParams{Email: r.Email, UpTo: rows[len(rows)-1].ID}); err != nil {
			log.Error().Err(err).Msg("portal notifications: cannot store the position")
			continue
		}
		sent++
	}
	return sent
}
