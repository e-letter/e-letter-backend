package mailer

import (
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Mailer is the interface the auth service depends on.
// Tests can swap in a mock without touching SMTP.
type Mailer interface {
	SendOTP(toEmail, otp string, expiresAt time.Time) error
}

// Config holds the SMTP credentials loaded from environment variables.
type Config struct {
	Host     string // e.g. smtp.gmail.com
	Port     string // e.g. 587
	Sender   string // EMAIL_SENDER — the From address
	Password string // EMAIL_PASSWORD — SMTP password / app-password
}

type smtpMailer struct {
	cfg Config
}

// New returns a production SMTP Mailer.
// If the sender address is empty the mailer falls back to console-only mode
// so the app boots even when email is not yet configured.
func New(cfg Config) Mailer {
	return &smtpMailer{cfg: cfg}
}

func (m *smtpMailer) SendOTP(toEmail, otp string, expiresAt time.Time) error {
	// Always log to console — useful during development.
	fmt.Printf("[EMAIL-OTP] To: %s | OTP: %s | Expires: %s\n",
		toEmail, otp, expiresAt.Format(time.RFC3339))

	// Skip actual SMTP delivery if credentials are not set.
	if m.cfg.Sender == "" || m.cfg.Password == "" || m.cfg.Host == "" {
		fmt.Println("[EMAIL-OTP] SMTP credentials not configured — OTP logged to console only.")
		return nil
	}

	subject := "Kode OTP Reset Password — E-Letter"
	body := buildOTPEmail(toEmail, otp, expiresAt)

	msg := "From: " + m.cfg.Sender + "\r\n" +
		"To: " + toEmail + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n" +
		"\r\n" +
		body

	addr := net.JoinHostPort(m.cfg.Host, m.cfg.Port)
	auth := smtp.PlainAuth("", m.cfg.Sender, m.cfg.Password, m.cfg.Host)

	if err := smtp.SendMail(addr, auth, m.cfg.Sender, []string{toEmail}, []byte(msg)); err != nil {
		return fmt.Errorf("gagal mengirim email OTP: %w", err)
	}
	return nil
}

// buildOTPEmail returns a minimal HTML email body containing the OTP.
func buildOTPEmail(toEmail, otp string, expiresAt time.Time) string {
	// Format expiry in WIB (UTC+7)
	loc, _ := time.LoadLocation("Asia/Jakarta")
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
              🔐 Reset Password
            </h1>
            <p style="margin:8px 0 0;color:#bfdbfe;font-size:13px;">E-Letter · SMK Negeri 2 Singosari</p>
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
              ⚠️ Jangan bagikan kode ini kepada siapapun termasuk pihak E-Letter.<br>
              Jika Anda tidak meminta reset password, abaikan email ini.
            </p>
          </td>
        </tr>
        <!-- Footer -->
        <tr>
          <td style="background:#f9fafb;padding:20px 40px;border-top:1px solid #e5e7eb;text-align:center;">
            <p style="margin:0;color:#9ca3af;font-size:12px;">
              © 2025 E-Letter · SMK Negeri 2 Singosari<br>
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
