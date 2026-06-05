package mailer

import (
	"fmt"
	"strings"
	"time"

	"github.com/resend/resend-go/v3"
)

type emailSender interface {
	Send(*resend.SendEmailRequest) (*resend.SendEmailResponse, error)
}

type resendSDKSender struct {
	client *resend.Client
}

func (s *resendSDKSender) Send(req *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
	return s.client.Emails.Send(req)
}

var newEmailSender = func(apiKey string) emailSender {
	return &resendSDKSender{client: resend.NewClient(apiKey)}
}

// Mailer is the interface the auth service depends on.
// Tests can swap in a mock without touching Resend.
type Mailer interface {
	SendOTP(toEmail, otp string, expiresAt time.Time) error
}

// Config holds the Resend credentials loaded from environment variables.
type Config struct {
	APIKey     string // RESEND_API_KEY
}

type resendMailer struct {
	cfg    Config
	sender emailSender
}

// New returns a production Resend mailer.
// If credentials are not set the mailer falls back to console-only mode so the
// app boots even when email delivery is not configured yet.
func New(cfg Config) Mailer {
	m := &resendMailer{cfg: cfg}
	if strings.TrimSpace(cfg.APIKey) != "" {
		m.sender = newEmailSender(cfg.APIKey)
	}
	return m
}

func (m *resendMailer) SendOTP(toEmail, otp string, expiresAt time.Time) error {
	toEmail = strings.TrimSpace(toEmail)
	otp = strings.TrimSpace(otp)

	if toEmail == "" {
		return fmt.Errorf("email tujuan OTP kosong")
	}
	if otp == "" {
		return fmt.Errorf("kode OTP kosong")
	}

	// Skip actual delivery if credentials are not set.
	if m.sender == nil {
		fmt.Printf("[EMAIL-OTP] Resend credentials not configured - email to %s skipped.\n", toEmail)
		return nil
	}

	fromAddress := "SiPena <sipena@resend.dev>"

	subject := "Kode OTP Reset Password - SiPena"
	body := buildOTPEmail(toEmail, otp, expiresAt)

	_, err := m.sender.Send(&resend.SendEmailRequest{
		From:    fromAddress,
		To:      []string{toEmail},
		Subject: subject,
		Html:    body,
	})
	if err != nil {
		return fmt.Errorf("gagal mengirim email OTP: %w", err)
	}
	return nil
}

// buildOTPEmail returns a minimal HTML email body containing the OTP.
func buildOTPEmail(toEmail, otp string, expiresAt time.Time) string {
	// Format expiry in WIB (UTC+7)
	loc := jakartaLocation()
	expStr := expiresAt.In(loc).Format("02 January 2006 15:04 WIB")
	_ = toEmail // available for personalisation if needed

	var sb strings.Builder
	sb.WriteString(`<!DOCTYPE html>
<html lang="id">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#f4f6fb;font-family:Arial,sans-serif;">
  <table width="100%" cellpadding="0" cellspacing="0" style="background:#f4f6fb;padding:40px 0;">
    <tr><td align="center">
      <table width="480" cellpadding="0" cellspacing="0"
             style="background:#ffffff;border-radius:12px;overflow:hidden;box-shadow:0 4px 24px rgba(0,0,0,.08);">
        <!-- Header -->
        <tr>
          <td style="background:#1e40af;padding:32px 40px 24px;text-align:center;">
            <h1 style="margin:0;color:#ffffff;font-size:22px;font-weight:700;letter-spacing:.5px;">
              🔐 Atur Ulang Kata Sandi
            </h1>
            <p style="margin:8px 0 0;color:#bfdbfe;font-size:13px;">SiPena · SMK Negeri 2 Singosari</p>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding:32px 40px;">
            <p style="margin:0 0 16px;color:#374151;font-size:15px;line-height:1.6;">
              Kami menerima permintaan reset password untuk akun Anda.<br>
              Gunakan kode OTP berikut untuk melanjutkan:
            </p>
            <!-- OTP Box -->
            <div style="background:#eff6ff;border:2px dashed #3b82f6;border-radius:10px;
                        text-align:center;padding:24px 16px;margin:24px 0;">
              <p style="margin:0 0 8px;color:#6b7280;font-size:12px;text-transform:uppercase;
                         letter-spacing:1px;font-weight:600;">Kode OTP Anda</p>
              <span style="font-size:40px;font-weight:800;color:#1e40af;letter-spacing:8px;">`)
	sb.WriteString(otp)
	sb.WriteString(`</span>
              <p style="margin:12px 0 0;color:#9ca3af;font-size:12px;">
                Berlaku hingga <strong style="color:#374151;">`)
	sb.WriteString(expStr)
	sb.WriteString(`</strong>
              </p>
            </div>
            <p style="margin:0 0 8px;color:#6b7280;font-size:13px;line-height:1.6;">
              ⚠️ Jangan bagikan kode ini kepada siapapun termasuk pihak SiPena.<br>
              Jika Anda tidak meminta reset password, abaikan email ini.
            </p>
          </td>
        </tr>
        <!-- Footer -->
        <tr>
          <td style="background:#f9fafb;padding:20px 40px;border-top:1px solid #e5e7eb;text-align:center;">
            <p style="margin:0;color:#9ca3af;font-size:12px;">
              © 2025 SiPena · SMK Negeri 2 Singosari<br>
              Email ini dikirim secara otomatis, mohon tidak membalas.
            </p>
          </td>
        </tr>
      </table>
    </td></tr>
  </table>
</body>
	</html>`)
	return sb.String()
}

func jakartaLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err == nil {
		return loc
	}
	return time.FixedZone("WIB", 7*60*60)
}
