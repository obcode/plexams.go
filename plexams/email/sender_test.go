package email

import (
	"strings"
	"testing"
)

func TestBuildMsgSetsFQDNMessageID(t *testing.T) {
	// Default hostname: Message-ID must use the FQDN, never os.Hostname() (e.g. docker-desktop).
	s := NewSender(SMTPConfig{PlanerEmail: "oliver.braun@hm.edu"})
	s.SetPlaner("Oliver Braun", "oliver.braun@hm.edu", "", "", "", "")
	msg, err := s.buildMsg([]string{"to@hm.edu"}, nil, "subj", []byte("body"), nil, nil, false)
	if err != nil {
		t.Fatalf("buildMsg: %v", err)
	}
	id := msg.GetMessageID()
	if !strings.HasSuffix(strings.TrimSuffix(id, ">"), "@"+defaultMailHostname) {
		t.Errorf("Message-ID = %q, want @%s domain", id, defaultMailHostname)
	}
	if strings.Contains(id, "docker") {
		t.Errorf("Message-ID = %q leaks the local hostname", id)
	}

	// Configured hostname overrides the default.
	s2 := NewSender(SMTPConfig{PlanerEmail: "x@hm.edu", Hostname: "custom.example.edu"})
	msg2, err := s2.buildMsg([]string{"to@hm.edu"}, nil, "subj", []byte("body"), nil, nil, false)
	if err != nil {
		t.Fatalf("buildMsg: %v", err)
	}
	if !strings.Contains(msg2.GetMessageID(), "@custom.example.edu>") {
		t.Errorf("Message-ID = %q, want @custom.example.edu", msg2.GetMessageID())
	}
}

func TestPlusPlexams(t *testing.T) {
	cases := map[string]string{
		"oliver.braun@hm.edu":     "oliver.braun+plexams@hm.edu",
		"oliver.braun+foo@hm.edu": "oliver.braun+plexams@hm.edu", // existing tag replaced
		"  oliver.braun@hm.edu  ": "oliver.braun+plexams@hm.edu", // trimmed
		"noreply@hm.edu":          "noreply+plexams@hm.edu",
		"":                        "",             // no addr
		"not-an-email":            "not-an-email", // no domain -> unchanged
	}
	for in, want := range cases {
		if got := plusPlexams(in); got != want {
			t.Errorf("plusPlexams(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEffectiveResolution_DefaultsFromPlanerEmail(t *testing.T) {
	s := NewSender(SMTPConfig{PlanerEmail: "oliver.braun@hm.edu"})
	s.SetPlaner("Oliver Braun", "oliver.braun@hm.edu", "", "", "", "")

	if got, want := s.DefaultMail(), "oliver.braun+plexams@hm.edu"; got != want {
		t.Errorf("DefaultMail = %q, want %q", got, want)
	}
	if got, want := s.EffectiveTestMail(), "oliver.braun+plexams@hm.edu"; got != want {
		t.Errorf("EffectiveTestMail = %q, want %q", got, want)
	}
	if got, want := s.EffectiveCc(), "oliver.braun+plexams@hm.edu"; got != want {
		t.Errorf("EffectiveCc = %q, want %q", got, want)
	}
	if got, want := s.EffectiveNoreplyMail(), defaultNoreplyMail; got != want {
		t.Errorf("EffectiveNoreplyMail = %q, want %q", got, want)
	}
	if got, want := s.EffectiveNoreplyName(), defaultNoreplyName; got != want {
		t.Errorf("EffectiveNoreplyName = %q, want %q", got, want)
	}
}

func TestEffectiveResolution_Precedence(t *testing.T) {
	// config values sit below the planner overrides and above the derived defaults.
	s := NewSender(SMTPConfig{
		PlanerEmail: "oliver.braun@hm.edu",
		TestMail:    "config-test@hm.edu",
		CC:          "config-cc@hm.edu",
		NoreplyMail: "config-noreply@hm.edu",
		NoreplyName: "Config Noreply",
	})

	// no override -> config wins over derived default
	s.SetPlaner("Oliver Braun", "oliver.braun@hm.edu", "", "", "", "")
	if got, want := s.EffectiveTestMail(), "config-test@hm.edu"; got != want {
		t.Errorf("config testMail: got %q, want %q", got, want)
	}
	if got, want := s.EffectiveNoreplyName(), "Config Noreply"; got != want {
		t.Errorf("config noreplyName: got %q, want %q", got, want)
	}

	// override wins over config
	s.SetPlaner("Oliver Braun", "oliver.braun@hm.edu", "ov-test@hm.edu", "ov-cc@hm.edu", "ov-noreply@hm.edu", "Override Noreply")
	if got, want := s.EffectiveTestMail(), "ov-test@hm.edu"; got != want {
		t.Errorf("override testMail: got %q, want %q", got, want)
	}
	if got, want := s.EffectiveCc(), "ov-cc@hm.edu"; got != want {
		t.Errorf("override cc: got %q, want %q", got, want)
	}
	if got, want := s.EffectiveNoreplyMail(), "ov-noreply@hm.edu"; got != want {
		t.Errorf("override noreplyMail: got %q, want %q", got, want)
	}
	if got, want := s.EffectiveNoreplyName(), "Override Noreply"; got != want {
		t.Errorf("override noreplyName: got %q, want %q", got, want)
	}
}

func TestDryRunOverride(t *testing.T) {
	s := NewSender(SMTPConfig{PlanerEmail: "oliver.braun@hm.edu"})
	s.SetPlaner("Oliver Braun", "oliver.braun@hm.edu", "", "", "", "")

	// no override -> dry run goes to the effective test mail
	if _, has := s.DryRunOverride(); has {
		t.Error("expected no override initially")
	}
	if got, want := s.DryRunRecipient(), "oliver.braun+plexams@hm.edu"; got != want {
		t.Errorf("DryRunRecipient default = %q, want %q", got, want)
	}

	// set override
	s.SetDryRunOverride("  temp@hm.edu ")
	if ov, has := s.DryRunOverride(); !has || ov != "temp@hm.edu" {
		t.Errorf("DryRunOverride = %q,%v, want temp@hm.edu,true", ov, has)
	}
	if got, want := s.DryRunRecipient(), "temp@hm.edu"; got != want {
		t.Errorf("DryRunRecipient override = %q, want %q", got, want)
	}

	// empty string clears it
	s.SetDryRunOverride("")
	if _, has := s.DryRunOverride(); has {
		t.Error("expected override cleared by empty string")
	}

	// reset also clears
	s.SetDryRunOverride("x@hm.edu")
	s.ResetDryRunOverride()
	if got, want := s.DryRunRecipient(), "oliver.braun+plexams@hm.edu"; got != want {
		t.Errorf("DryRunRecipient after reset = %q, want %q", got, want)
	}
}
