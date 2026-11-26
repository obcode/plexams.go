package email

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/obcode/plexams.go/plexams/apierr"
	"github.com/rs/zerolog/log"
	"github.com/wneessen/go-mail"
)

// Waits between connection attempts. Only the connection is retried, never the
// transmission: once the server has seen MAIL FROM a second attempt could deliver the mail
// twice, and a mail that was refused (auth, recipient, content) is refused again.
//
// The nightly auto-sync report has no one waiting for it and failed twice on a mail server
// that was briefly unreachable (no route to host, dial timeout), so it waits longer. A send
// started from the GUI keeps its retries short, because the request is open meanwhile.
var (
	interactiveDialRetryDelays = []time.Duration{5 * time.Second, 20 * time.Second}
	systemDialRetryDelays      = []time.Duration{1 * time.Minute, 5 * time.Minute}
)

// sleepCtx waits for d or until ctx is done. Replaced in tests.
var sleepCtx = func(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// isTransientDialError reports whether a failed connection attempt is worth repeating: the
// network did not get us to the server (no route, refused, timeout). An answer from the
// server itself, such as a rejected login or a 554 greeting, is not.
func isTransientDialError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}

// dialAndSend connects, retrying transient connection failures after the given delays, then
// sends msg once and closes the connection. Errors read like go-mail's DialAndSend.
func dialAndSend(ctx context.Context, client *mail.Client, delays []time.Duration, msg *mail.Msg) (err error) {
	smtpClient, err := client.DialToSMTPClientWithContext(ctx)
	for attempt := 0; err != nil && attempt < len(delays) && isTransientDialError(err); attempt++ {
		log.Warn().Err(err).Int("attempt", attempt+1).Dur("retry_in", delays[attempt]).
			Msg("smtp: cannot connect, retrying")
		if sleepErr := sleepCtx(ctx, delays[attempt]); sleepErr != nil {
			break
		}
		smtpClient, err = client.DialToSMTPClientWithContext(ctx)
	}
	if err != nil {
		return &smtpFailure{
			coded: apierr.New(apierr.CodeExternalSMTP, "keine Verbindung zum Mailserver, diese E-Mail wurde nicht verschickt"),
			cause: err,
		}
	}
	defer func() {
		if closeErr := client.CloseWithSMTPClient(smtpClient); closeErr != nil {
			// The mail went out or the send already failed; a failing QUIT changes
			// neither, and reporting it as a failure would invite a duplicate send.
			log.Warn().Err(closeErr).Msg("smtp: cannot close connection")
		}
	}()
	if err = client.SendWithSMTPClient(smtpClient, msg); err != nil {
		return &smtpFailure{
			coded: apierr.New(apierr.CodeExternalSMTP, "der Mailserver hat diese E-Mail abgelehnt (die genaue Antwort steht im Server-Log)"),
			cause: err,
		}
	}
	return nil
}

// smtpFailure is a failed connection or send: the planner reads the German, coded
// message; the mail server's own answer stays in the chain for the log.
type smtpFailure struct {
	coded *apierr.Error
	cause error
}

func (e *smtpFailure) Error() string   { return e.coded.Message }
func (e *smtpFailure) Unwrap() []error { return []error{e.coded, e.cause} }
