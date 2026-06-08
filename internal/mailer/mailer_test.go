package mailer

import (
	"strings"
	"testing"
	"time"

	"github.com/resend/resend-go/v3"
)

func TestBuildOTPEmail(t *testing.T) {
	expiresAt := time.Date(2026, 6, 4, 14, 30, 0, 0, time.UTC)

	body := buildOTPEmail("user@example.com", "123456", expiresAt)

	if !strings.Contains(body, "123456") {
		t.Fatalf("expected OTP to be present in body, got %q", body)
	}
	if !strings.Contains(body, "04 June 2026 21:30 WIB") {
		t.Fatalf("expected WIB expiry to be rendered, got %q", body)
	}
}

func TestSendOTPWithoutResendConfigSkipsDelivery(t *testing.T) {
	called := false
	mailer := &resendMailer{cfg: Config{}}

	if err := mailer.SendOTP("user@example.com", "123456", time.Now()); err != nil {
		t.Fatalf("expected no error when Resend is not configured, got %v", err)
	}
	if called {
		t.Fatal("expected Resend send to be skipped when config is incomplete")
	}
}

func TestSendOTPSendsResendMessage(t *testing.T) {
	var captured *resend.SendEmailRequest

	mailer := &resendMailer{
		cfg: Config{
			APIKey: "re_test_key",
		},
		sender: &stubSender{
			sendFn: func(req *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
				captured = req
				return nil, nil
			},
		},
	}

	expiresAt := time.Date(2026, 6, 4, 14, 30, 0, 0, time.UTC)
	if err := mailer.SendOTP(" user@example.com ", " 654321 ", expiresAt); err != nil {
		t.Fatalf("expected send to succeed, got %v", err)
	}

	if captured == nil {
		t.Fatal("expected resend payload to be captured")
	}
	if captured.From != "SiPena <sipena@smkn2singosari.sch.id>" {
		t.Fatalf("unexpected from address: %s", captured.From)
	}
	if len(captured.To) != 1 || captured.To[0] != "user@example.com" {
		t.Fatalf("unexpected recipient list: %#v", captured.To)
	}
	if captured.Subject != "Kode OTP Reset Password - SiPena" {
		t.Fatalf("unexpected subject: %s", captured.Subject)
	}

	if !strings.Contains(captured.Html, "654321") {
		t.Fatalf("expected OTP to be present in html, got %q", captured.Html)
	}
	if !strings.Contains(captured.Html, "04 June 2026 21:30 WIB") {
		t.Fatalf("expected expiry to be present in html, got %q", captured.Html)
	}
}

func TestSendOTPRejectsEmptyValues(t *testing.T) {
	mailer := &resendMailer{cfg: Config{APIKey: "re_test_key"}}

	if err := mailer.SendOTP("   ", "123456", time.Now()); err == nil {
		t.Fatal("expected error for empty recipient")
	}
	if err := mailer.SendOTP("user@example.com", "   ", time.Now()); err == nil {
		t.Fatal("expected error for empty OTP")
	}
}

func TestJakartaLocationFallback(t *testing.T) {
	loc := jakartaLocation()
	if loc == nil {
		t.Fatal("expected location to be returned")
	}

	_, offset := time.Now().In(loc).Zone()
	if offset != 7*60*60 {
		t.Fatalf("expected WIB offset, got %d", offset)
	}
}

type stubSender struct {
	sendFn func(*resend.SendEmailRequest) (*resend.SendEmailResponse, error)
}

func (s *stubSender) Send(req *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
	if s.sendFn == nil {
		return nil, nil
	}
	return s.sendFn(req)
}
