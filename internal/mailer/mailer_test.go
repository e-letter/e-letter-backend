package mailer

import (
	"fmt"
	"net/smtp"
	"strings"
	"testing"
	"time"
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

func TestSendOTPWithoutSMTPConfigSkipsDelivery(t *testing.T) {
	originalSendMail := sendMail
	defer func() { sendMail = originalSendMail }()

	called := false
	sendMail = func(string, smtp.Auth, string, []string, []byte) error {
		called = true
		return nil
	}

	mailer := &smtpMailer{cfg: Config{}}
	if err := mailer.SendOTP("user@example.com", "123456", time.Now()); err != nil {
		t.Fatalf("expected no error when SMTP is not configured, got %v", err)
	}
	if called {
		t.Fatal("expected SMTP send to be skipped when config is incomplete")
	}
}

func TestSendOTPSendsSMTPMessage(t *testing.T) {
	originalSendMail := sendMail
	defer func() { sendMail = originalSendMail }()

	var (
		addr   string
		from   string
		toList []string
		rawMsg []byte
	)

	sendMail = func(a string, auth smtp.Auth, f string, to []string, msg []byte) error {
		if got := fmt.Sprintf("%T", auth); !strings.Contains(got, "plainAuth") {
			t.Fatalf("expected plain auth, got %s", got)
		}
		addr = a
		from = f
		toList = append([]string(nil), to...)
		rawMsg = append([]byte(nil), msg...)
		return nil
	}

	mailer := &smtpMailer{cfg: Config{
		Host:     "smtp.example.com",
		Port:     "587",
		Sender:   "noreply@example.com",
		Password: "secret",
	}}

	expiresAt := time.Date(2026, 6, 4, 14, 30, 0, 0, time.UTC)
	if err := mailer.SendOTP(" user@example.com ", " 654321 ", expiresAt); err != nil {
		t.Fatalf("expected send to succeed, got %v", err)
	}

	if addr != "smtp.example.com:587" {
		t.Fatalf("unexpected SMTP addr: %s", addr)
	}
	if from != "noreply@example.com" {
		t.Fatalf("unexpected from address: %s", from)
	}
	if len(toList) != 1 || toList[0] != "user@example.com" {
		t.Fatalf("unexpected recipient list: %#v", toList)
	}

	msg := string(rawMsg)
	if !strings.Contains(msg, "Subject: Kode OTP Reset Password") {
		t.Fatalf("expected subject header in message, got %q", msg)
	}
	if !strings.Contains(msg, "654321") {
		t.Fatalf("expected OTP to be present in message, got %q", msg)
	}
	if !strings.Contains(msg, "04 June 2026 21:30 WIB") {
		t.Fatalf("expected expiry to be present in message, got %q", msg)
	}
}

func TestSendOTPRejectsEmptyValues(t *testing.T) {
	mailer := &smtpMailer{cfg: Config{
		Host:     "smtp.example.com",
		Port:     "587",
		Sender:   "noreply@example.com",
		Password: "secret",
	}}

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

	// The fallback uses WIB offset when the zone database is unavailable.
	_, offset := time.Now().In(loc).Zone()
	if offset != 7*60*60 {
		t.Fatalf("expected WIB offset, got %d", offset)
	}
}
