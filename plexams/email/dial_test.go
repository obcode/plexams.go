package email

import (
	"bufio"
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeSMTP returns a Sender whose dials go to dial instead of the network, and records the
// waits between attempts instead of sleeping.
func fakeSMTP(t *testing.T, dial func() (net.Conn, error)) (s *Sender, dials *int, waits *[]time.Duration) {
	t.Helper()
	dials, waits = new(int), new([]time.Duration)
	orig := sleepCtx
	sleepCtx = func(_ context.Context, d time.Duration) error {
		*waits = append(*waits, d)
		return nil
	}
	t.Cleanup(func() { sleepCtx = orig })

	s = NewSender(SMTPConfig{Server: "smtp.example.org", Port: 587, PlanerName: "P", PlanerEmail: "p@example.org"})
	s.SetPlaner("P", "p@example.org", "", "", "", "")
	s.dialContext = func(context.Context, string, string) (net.Conn, error) {
		*dials++
		return dial()
	}
	return s, dials, waits
}

func unreachable() (net.Conn, error) {
	return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.EHOSTUNREACH}
}

func TestSystemMailRetriesUnreachableServerWithLongWaits(t *testing.T) {
	s, dials, waits := fakeSMTP(t, unreachable)

	err := s.SendSystem(true, []string{"to@example.org"}, "subj", []byte("body"), nil, nil)

	if err == nil || !strings.HasPrefix(err.Error(), "dial failed: ") {
		t.Fatalf("err = %v, want a dial failure", err)
	}
	if *dials != 3 {
		t.Errorf("dials = %d, want 3", *dials)
	}
	if !reflect.DeepEqual(*waits, systemDialRetryDelays) {
		t.Errorf("waits = %v, want %v", *waits, systemDialRetryDelays)
	}
}

func TestInteractiveMailRetriesWithShortWaits(t *testing.T) {
	s, dials, waits := fakeSMTP(t, unreachable)

	if err := s.Send(true, []string{"to@example.org"}, nil, "subj", []byte("body"), nil, nil, false); err == nil {
		t.Fatal("want an error")
	}
	if *dials != 3 {
		t.Errorf("dials = %d, want 3", *dials)
	}
	if !reflect.DeepEqual(*waits, interactiveDialRetryDelays) {
		t.Errorf("waits = %v, want %v", *waits, interactiveDialRetryDelays)
	}
}

func TestNoRetryWhenTheServerAnswers(t *testing.T) {
	// The server is reachable and refuses: repeating would only be refused again.
	s, dials, waits := fakeSMTP(t, func() (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer func() { _ = server.Close() }()
			_, _ = server.Write([]byte("554 5.7.1 not now\r\n"))
			_, _ = bufio.NewReader(server).ReadString('\n')
		}()
		return client, nil
	})

	err := s.SendSystem(true, []string{"to@example.org"}, "subj", []byte("body"), nil, nil)

	if err == nil || !strings.Contains(err.Error(), "554") {
		t.Fatalf("err = %v, want the server's 554", err)
	}
	if *dials != 1 || len(*waits) != 0 {
		t.Errorf("dials = %d, waits = %v; want one dial and no wait", *dials, *waits)
	}
}

func TestIsTransientDialError(t *testing.T) {
	timeout := &net.OpError{Op: "dial", Net: "tcp", Err: &timeoutErr{}}
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"no route to host", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.EHOSTUNREACH}, true},
		{"dial timeout", timeout, true},
		{"wrapped", errors.Join(errors.New("ctx"), timeout), true},
		{"server reply", errors.New("535 5.7.8 authentication failed"), false},
	} {
		if got := isTransientDialError(tc.err); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

type timeoutErr struct{}

func (*timeoutErr) Error() string   { return "i/o timeout" }
func (*timeoutErr) Timeout() bool   { return true }
func (*timeoutErr) Temporary() bool { return true }
